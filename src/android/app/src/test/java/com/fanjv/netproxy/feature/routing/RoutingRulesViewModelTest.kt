package com.fanjv.netproxy.feature.routing

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.feature.routing.model.LocalRuleDocument
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.simpleRule
import com.fanjv.netproxy.feature.routing.presentation.RuleIdentity
import com.fanjv.netproxy.feature.routing.presentation.RoutingRulesState
import com.fanjv.netproxy.feature.routing.presentation.RoutingRulesViewModel
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class RoutingRulesViewModelTest {
    @get:Rule val folder = TemporaryFolder()

    private class Transport : TestApiTransport() {
        var content = """{"version":1,"rules":[{"domain_suffix":"example.com"},{"domain":"advanced.example","port":443}]}"""
        var revision = "read-revision"
        var applyError: ModuleApiException? = null
        var readError: Exception? = null
        val contents = mutableMapOf<String, String>()
        val revisions = mutableMapOf<String, String>()
        val readErrors = mutableMapOf<String, Exception>()
        var gate: CountDownLatch? = null
        val entered = CompletableDeferred<Unit>()
        val calls = mutableListOf<ApiTestRequest>()

        override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
            calls += request
            val data = when (request.method) {
                "GET" -> {
                    (readErrors[request.target] ?: readError)?.let { throw it }
                    JsonObject(mapOf("content" to JsonPrimitive(contents[request.target] ?: content),
                        "revision" to JsonPrimitive(revisions[request.target] ?: revision)))
                }
                "PUT" -> {
                    gate?.let { entered.complete(Unit); check(it.await(5, TimeUnit.SECONDS)) }
                    applyError?.let { throw it }
                    val target = request.target
                    assertEquals(revisions[target] ?: revision, request.text("revision"))
                    contents[target] = request.text("content")
                    revisions[target] = "saved-revision"
                    JsonObject(mapOf("revision" to JsonPrimitive("saved-revision")))
                }
                else -> error("Unexpected command")
            }
            return apiSuccess(data.toString())
        }
    }

    private fun model(scope: CoroutineScope, transport: Transport) = RoutingRulesViewModel(
        ConfigRepository(ModuleApiClient(transport = transport)), scope)

    private fun RoutingRulesState.file(target: LocalRuleSet = LocalRuleSet.Proxy) = files.getValue(target)

    private suspend fun RoutingRulesViewModel.loaded(): RoutingRulesState = withTimeout(5_000) {
        state.first { !it.isLoading && it.files.values.all { file -> file.document != null || file.requiresReload } }
    }

    private suspend fun RoutingRulesViewModel.saved(): RoutingRulesState = withTimeout(5_000) {
        state.first { !it.isSaving }
    }

    @Test fun initialLoadMergesAllGroupsWithDistinctFileAndIndexIdentities() = runBlocking {
        val transport = Transport()
        LocalRuleSet.entries.forEach { target ->
            transport.contents[target.target] = """{"version":1,"rules":[{"domain":"${target.tag}.example"}]}"""
            transport.revisions[target.target] = "${target.tag}-revision"
        }
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        assertEquals(LocalRuleSet.entries.map { it.target }, transport.calls.map { it.target })
        assertEquals(LocalRuleSet.entries.map { RuleIdentity(it, 0) }, loaded.rules)
        assertEquals(3, loaded.rules.distinct().size)
        assertTrue(loaded.hasDocuments)
        assertFalse(loaded.isEmpty)
        LocalRuleSet.entries.forEach { target ->
            assertTrue(loaded.editable(target))
            assertEquals("${target.tag}-revision", loaded.file(target).revision)
            assertEquals(LocalRuleDocument.parse(transport.contents.getValue(target.target)), loaded.file(target).document)
        }
        vm.add()
        LocalRuleSet.entries.forEach(vm::selectTarget)
        vm.dismiss()
        assertEquals(3, transport.calls.size)
    }

    @Test fun newDraftSelectsAnyTargetWithoutReadingAndSavesOnlyThatFile() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        LocalRuleSet.entries.forEach { target ->
            val before = vm.state.value.files
            vm.add()
            vm.updateDraft(RuleField.Port, "443\n8443")
            val calls = transport.calls.size
            vm.selectTarget(target)
            assertEquals(target, vm.state.value.draft!!.target)
            assertEquals("443\n8443", vm.state.value.draft!!.text)
            assertEquals(RuleField.Port, vm.state.value.draft!!.field)
            assertNull(vm.state.value.draft!!.index)
            assertEquals(calls, transport.calls.size)
            vm.save()
            val saved = vm.saved()
            assertNull(saved.draft)
            assertEquals("saved-revision", saved.file(target).revision)
            val document = saved.file(target).document!!
            assertEquals(3, document.rules.size)
            assertEquals(before.getValue(target).document!!.rules.toList(), document.rules.take(2))
            assertEquals(target.target, transport.calls.last().target)
            LocalRuleSet.entries.filter { it != target }.forEach { other ->
                assertEquals(before.getValue(other), saved.file(other))
            }
        }
        assertEquals(3, transport.calls.count { it.method == "PUT" })
        assertFalse(transport.calls.any { it.path != "/config" })
    }

    @Test fun editingAndDeletingUseFileIdentityAndCannotMoveExistingRules() = runBlocking {
        val transport = Transport()
        transport.contents[LocalRuleSet.Proxy.target] = """{"version":1,"rules":[{"domain":"proxy.example"}]}"""
        transport.revisions[LocalRuleSet.Direct.target] = "direct-revision"
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        val direct = RuleIdentity(LocalRuleSet.Direct, 0)
        val advanced = loaded.file(LocalRuleSet.Direct).document!!.rules[1]
        vm.edit(direct)
        assertEquals("example.com", vm.state.value.draft!!.text)
        vm.selectTarget(LocalRuleSet.Block)
        assertEquals(LocalRuleSet.Direct, vm.state.value.draft!!.target)
        vm.updateDraft(RuleField.Domain, "direct.example")
        vm.save()
        val saved = vm.saved()
        assertEquals("direct.example", saved.file(LocalRuleSet.Direct).document!!.rules[0].simpleRule()!!.values.single())
        assertEquals(advanced, saved.file(LocalRuleSet.Direct).document!!.rules[1])
        assertEquals(Triple("PUT", LocalRuleSet.Direct.target, "direct-revision"), transport.calls.last().let { Triple(it.method, it.target, it.text("revision")) })
        vm.requestDelete(direct)
        assertEquals(direct, vm.state.value.deleting)
        vm.delete()
        val deleted = vm.saved()
        assertEquals(listOf(advanced), deleted.file(LocalRuleSet.Direct).document!!.rules.toList())
        assertEquals(loaded.file(LocalRuleSet.Proxy), deleted.file(LocalRuleSet.Proxy))
        assertEquals(loaded.file(LocalRuleSet.Block), deleted.file(LocalRuleSet.Block))
        assertEquals(advanced, LocalRuleDocument.parse(transport.contents.getValue(LocalRuleSet.Direct.target)).rules.single())
    }

    @Test fun partialFailureKeepsHealthyRulesAndPerFileErrorsAndRefreshRecovers() = runBlocking {
        val transport = Transport()
        transport.readErrors[LocalRuleSet.Direct.target] = ModuleApiException("config.read_failed", "direct read failed")
        transport.contents[LocalRuleSet.Block.target] = "{}"
        val vm = model(this, transport)
        vm.refresh()
        val failed = vm.loaded()
        assertEquals(listOf(RuleIdentity(LocalRuleSet.Proxy, 0), RuleIdentity(LocalRuleSet.Proxy, 1)), failed.rules)
        assertTrue(failed.canAdd)
        assertTrue(failed.editable(LocalRuleSet.Proxy))
        assertEquals("direct read failed", failed.file(LocalRuleSet.Direct).error)
        assertTrue(failed.file(LocalRuleSet.Direct).requiresReload)
        assertNull(failed.file(LocalRuleSet.Direct).document)
        assertFalse(failed.editable(LocalRuleSet.Direct))
        assertTrue(failed.file(LocalRuleSet.Block).invalidDocument)
        assertFalse(failed.editable(LocalRuleSet.Block))
        vm.add()
        vm.selectTarget(LocalRuleSet.Direct)
        vm.updateDraft(RuleField.Domain, "new.example")
        vm.save()
        assertEquals("direct read failed", vm.state.value.file(LocalRuleSet.Direct).error)
        assertFalse(transport.calls.any { it.method == "PUT" })
        vm.selectTarget(LocalRuleSet.Block)
        vm.save()
        assertFalse(transport.calls.any { it.method == "PUT" })
        vm.dismiss()
        transport.readErrors.clear()
        transport.contents.clear()
        vm.refresh()
        val recovered = vm.loaded()
        LocalRuleSet.entries.forEach { target ->
            assertTrue(recovered.editable(target))
            assertFalse(recovered.file(target).invalidDocument)
            assertFalse(recovered.file(target).requiresReload)
            assertEquals("", recovered.file(target).error)
        }
    }

    @Test fun healthyFileCanSaveWhileOtherFilesRemainFailed() = runBlocking {
        val transport = Transport()
        transport.readErrors[LocalRuleSet.Proxy.target] = ModuleApiException("config.read_failed", "proxy read failed")
        transport.contents[LocalRuleSet.Block.target] = "{}"
        val vm = model(this, transport)
        vm.refresh()
        val failed = vm.loaded()
        vm.add()
        assertEquals(LocalRuleSet.Direct, vm.state.value.draft!!.target)
        vm.updateDraft(RuleField.Domain, "new.example")
        vm.save()
        val saved = vm.saved()
        assertEquals(LocalRuleSet.Direct.target, transport.calls.last().target)
        assertEquals(failed.file(LocalRuleSet.Proxy), saved.file(LocalRuleSet.Proxy))
        assertEquals(failed.file(LocalRuleSet.Block), saved.file(LocalRuleSet.Block))
        assertFalse(saved.isEmpty)
    }

    @Test fun unknownAndFailedFilesAreNotEmptyLists() = runBlocking {
        val transport = Transport()
        transport.content = """{"version":1,"rules":[]}"""
        val vm = model(this, transport)
        assertFalse(vm.state.value.isEmpty)
        assertFalse(vm.state.value.hasDocuments)
        vm.refresh()
        assertTrue(vm.loaded().isEmpty)
        transport.readErrors[LocalRuleSet.Direct.target] = ModuleApiException("config.read_failed", "direct read failed")
        vm.refresh()
        val partial = vm.loaded()
        assertTrue(partial.hasDocuments)
        assertTrue(partial.rules.isEmpty())
        assertFalse(partial.isEmpty)
        transport.readError = ModuleApiException("config.read_failed", "read failed")
        val fresh = model(this, transport)
        fresh.refresh()
        val failed = fresh.loaded()
        assertFalse(failed.hasDocuments)
        assertFalse(failed.isEmpty)
        assertFalse(failed.canAdd)
        assertTrue(failed.files.values.all { it.requiresReload && it.error.isNotEmpty() })
        fresh.add()
        assertNull(fresh.state.value.draft)
    }

    @Test fun refreshUpdatesEveryGroupAfterExternalJsonEdit() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        val external = """{"version":5,"rules":[{"port":443}]}"""
        transport.contents[LocalRuleSet.Block.target] = external
        transport.revisions[LocalRuleSet.Block.target] = "external-revision"
        vm.refresh()
        val loaded = vm.loaded()
        assertEquals("external-revision", loaded.file(LocalRuleSet.Block).revision)
        assertEquals(LocalRuleDocument.parse(external), loaded.file(LocalRuleSet.Block).document)
        assertEquals(6, transport.calls.size)
    }

    @Test fun conflictRetainsTargetDraftAndOriginalRevisionUntilExplicitReload() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.edit(RuleIdentity(LocalRuleSet.Direct, 0))
        vm.updateDraft(RuleField.DomainSuffix, "new.example")
        transport.applyError = ModuleApiException("config.conflict", "changed")
        transport.revisions[LocalRuleSet.Direct.target] = "external-revision"
        vm.save()
        val failed = vm.saved()
        assertTrue(failed.file(LocalRuleSet.Direct).requiresReload)
        assertEquals("read-revision", failed.file(LocalRuleSet.Direct).revision)
        assertEquals("new.example", failed.draft!!.text)
        assertEquals(LocalRuleSet.Direct, failed.draft.target)
        assertFalse(failed.file(LocalRuleSet.Proxy).requiresReload)
        val calls = transport.calls.size
        vm.updateDraft(RuleField.DomainSuffix, "corrected.example")
        vm.selectTarget(LocalRuleSet.Block)
        vm.save()
        vm.refresh()
        assertEquals(LocalRuleSet.Direct, vm.state.value.draft!!.target)
        assertEquals("changed", vm.state.value.file(LocalRuleSet.Direct).error)
        assertEquals(calls, transport.calls.size)
        vm.refresh(discardDraft = true)
        assertEquals("external-revision", vm.loaded().file(LocalRuleSet.Direct).revision)
        assertNull(vm.state.value.draft)
    }

    @Test fun validationFailureKeepsDraftAndAllowsCorrectionOnlyWithUnchangedTargetRevision() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.add()
        vm.selectTarget(LocalRuleSet.Block)
        vm.updateDraft(RuleField.DomainRegex, "[")
        transport.applyError = ModuleApiException("command.failed", "invalid regex")
        vm.save()
        val failed = vm.saved()
        assertFalse(failed.file(LocalRuleSet.Block).requiresReload)
        assertEquals("invalid regex", failed.file(LocalRuleSet.Block).error)
        assertEquals("[", failed.draft!!.text)
        assertEquals("GET", transport.calls.last().method)
        assertEquals(LocalRuleSet.Block.target, transport.calls.last().target)
        transport.applyError = null
        vm.updateDraft(RuleField.DomainRegex, "^example\\.com$")
        vm.save()
        assertNull(vm.saved().draft)
    }

    @Test fun saveErrorBelongsToActiveDialogAndReturnsToPageAfterDismissal() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.add()
        vm.selectTarget(LocalRuleSet.Direct)
        vm.updateDraft(RuleField.IpCidr, "192.168.10.0/24")
        transport.applyError = ModuleApiException("command.failed", "reload failed")
        vm.save()
        val failed = vm.saved()
        assertEquals("reload failed", failed.file(LocalRuleSet.Direct).error)
        assertFalse(failed.showPageError(LocalRuleSet.Direct))
        vm.dismiss()
        assertTrue(vm.state.value.showPageError(LocalRuleSet.Direct))
        vm.requestDelete(RuleIdentity(LocalRuleSet.Direct, 0))
        vm.delete()
        assertFalse(vm.saved().showPageError(LocalRuleSet.Direct))
        vm.dismiss()
        assertTrue(vm.state.value.showPageError(LocalRuleSet.Direct))
    }

    @Test fun packageRulesSaveThroughExistingRevisionCheckedConfigurationEntry() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.add()
        vm.selectTarget(LocalRuleSet.Direct)
        vm.updateDraft(RuleField.PackageName, "com.android.chrome\ncom.example.app")
        vm.save()
        val saved = vm.saved()
        assertNull(saved.draft)
        val rule = saved.file(LocalRuleSet.Direct).document!!.rules.last().simpleRule()!!
        assertEquals(RuleField.PackageName, rule.field)
        assertEquals(listOf("com.android.chrome", "com.example.app"), rule.values)
        assertEquals(Triple("PUT", LocalRuleSet.Direct.target, "read-revision"), transport.calls.last().let { Triple(it.method, it.target, it.text("revision")) })
    }

    @Test fun nonConflictFailureWithChangedRevisionCannotRetryWithNewRevision() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.add()
        vm.selectTarget(LocalRuleSet.Block)
        vm.updateDraft(RuleField.Domain, "new.example")
        transport.applyError = ModuleApiException("command.failed", "failed")
        transport.revisions[LocalRuleSet.Block.target] = "external-revision"
        vm.save()
        val failed = vm.saved()
        assertTrue(failed.file(LocalRuleSet.Block).requiresReload)
        assertEquals("read-revision", failed.file(LocalRuleSet.Block).revision)
        val calls = transport.calls.size
        transport.applyError = null
        vm.save()
        assertEquals(calls, transport.calls.size)
    }

    @Test fun inFlightSaveBlocksRefreshTargetChangeDuplicateWritesAndDismissal() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        transport.gate = CountDownLatch(1)
        vm.add()
        vm.selectTarget(LocalRuleSet.Direct)
        vm.updateDraft(RuleField.Domain, "new.example")
        vm.save()
        try {
            withTimeout(5_000) { transport.entered.await() }
            vm.refresh(discardDraft = true)
            vm.selectTarget(LocalRuleSet.Block)
            vm.updateDraft(RuleField.Port, "8443")
            vm.requestDelete(RuleIdentity(LocalRuleSet.Proxy, 0))
            vm.dismiss()
            vm.save()
            assertEquals(LocalRuleSet.Direct, vm.state.value.draft!!.target)
            assertEquals("new.example", vm.state.value.draft!!.text)
            assertEquals(1, transport.calls.count { it.method == "PUT" })
            assertEquals(3, transport.calls.count { it.method == "GET" })
        } finally {
            transport.gate!!.countDown()
        }
        assertEquals("saved-revision", vm.saved().file(LocalRuleSet.Direct).revision)
    }

    @Test fun failedRevisionCheckKeepsOriginalSnapshotAndDraftNonWritable() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        vm.edit(RuleIdentity(LocalRuleSet.Block, 0))
        vm.updateDraft(RuleField.Domain, "new.example")
        transport.applyError = ModuleApiException("command.failed", "failed")
        transport.readErrors[LocalRuleSet.Block.target] = ModuleApiException("config.read_failed", "read failed")
        vm.save()
        val failed = vm.saved()
        assertTrue(failed.file(LocalRuleSet.Block).requiresReload)
        assertEquals(loaded.file(LocalRuleSet.Block).revision, failed.file(LocalRuleSet.Block).revision)
        assertEquals(loaded.file(LocalRuleSet.Block).document, failed.file(LocalRuleSet.Block).document)
        assertEquals("new.example", failed.draft!!.text)
        val calls = transport.calls.size
        vm.save()
        assertEquals(calls, transport.calls.size)
    }

    @Test fun deleteConflictRetainsIdentityAndDoesNotTouchOtherFiles() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        val identity = RuleIdentity(LocalRuleSet.Block, 0)
        vm.requestDelete(identity)
        transport.applyError = ModuleApiException("config.conflict", "changed")
        vm.delete()
        val failed = vm.saved()
        assertEquals(identity, failed.deleting)
        assertTrue(failed.file(LocalRuleSet.Block).requiresReload)
        assertEquals(loaded.file(LocalRuleSet.Block).revision, failed.file(LocalRuleSet.Block).revision)
        assertEquals(loaded.file(LocalRuleSet.Block).document, failed.file(LocalRuleSet.Block).document)
        assertEquals(loaded.file(LocalRuleSet.Proxy), failed.file(LocalRuleSet.Proxy))
        assertEquals(loaded.file(LocalRuleSet.Direct), failed.file(LocalRuleSet.Direct))
        val calls = transport.calls.size
        vm.delete()
        vm.refresh()
        assertEquals(calls, transport.calls.size)
    }

    @Test fun advancedRulesCannotBeConvertedByFormAndMalformedDocumentIsNotReplaced() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        vm.edit(RuleIdentity(LocalRuleSet.Block, 1))
        assertNull(vm.state.value.draft!!.field)
        assertEquals(LocalRuleSet.Block, vm.state.value.draft!!.target)
        vm.selectTarget(LocalRuleSet.Proxy)
        vm.updateDraft(RuleField.Domain, "new.example")
        vm.save()
        assertNull(vm.state.value.draft!!.field)
        assertEquals(LocalRuleSet.Block, vm.state.value.draft!!.target)
        assertEquals(loaded.files, vm.state.value.files)
        assertFalse(transport.calls.any { it.method == "PUT" })
        vm.dismiss()
        transport.contents[LocalRuleSet.Block.target] = "{}"
        vm.refresh()
        val invalid = vm.loaded()
        assertTrue(invalid.file(LocalRuleSet.Block).invalidDocument)
        assertNull(invalid.file(LocalRuleSet.Block).document)
        vm.edit(RuleIdentity(LocalRuleSet.Block, 0))
        assertNull(vm.state.value.draft)
        assertFalse(transport.calls.any { it.method == "PUT" })
    }

    @Test fun deleteFromEditorClosesDraftAndCancellationDoesNotWrite() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        val identity = RuleIdentity(LocalRuleSet.Direct, 0)
        vm.edit(identity)
        vm.updateDraft(RuleField.DomainSuffix, "unsaved.example")
        vm.requestDelete(identity)
        assertNull(vm.state.value.draft)
        assertEquals(identity, vm.state.value.deleting)
        vm.dismiss()
        assertNull(vm.state.value.deleting)
        assertEquals(loaded.files, vm.state.value.files)
        assertFalse(transport.calls.any { it.method == "PUT" })
    }

    @Test fun failedRefreshRetainsDisplayedDataButOnlyFailedFileRequiresReload() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        transport.readErrors[LocalRuleSet.Direct.target] = ModuleApiException("config.read_failed", "read failed")
        vm.refresh()
        val failed = vm.loaded()
        assertEquals(loaded.rules, failed.rules)
        assertEquals(loaded.file(LocalRuleSet.Direct).document, failed.file(LocalRuleSet.Direct).document)
        assertFalse(failed.editable(LocalRuleSet.Direct))
        assertEquals("read failed", failed.file(LocalRuleSet.Direct).error)
        assertTrue(failed.editable(LocalRuleSet.Proxy))
        assertTrue(failed.editable(LocalRuleSet.Block))
        vm.edit(RuleIdentity(LocalRuleSet.Direct, 0))
        assertNull(vm.state.value.draft)
        vm.requestDelete(RuleIdentity(LocalRuleSet.Direct, 0))
        assertNull(vm.state.value.deleting)
    }
}
