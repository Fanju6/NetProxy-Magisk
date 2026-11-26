package com.fanjv.netproxy.feature.routing

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.routing.model.LocalRuleDocument
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.policyRule
import com.fanjv.netproxy.feature.routing.model.simpleRule
import com.fanjv.netproxy.feature.routing.presentation.RuleIdentity
import com.fanjv.netproxy.feature.routing.presentation.RuleSection
import com.fanjv.netproxy.feature.routing.presentation.RoutingRulesState
import com.fanjv.netproxy.feature.routing.presentation.RoutingRulesViewModel
import com.fanjv.netproxy.feature.routing.presentation.groupedRules
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
        var readGate: CompletableDeferred<Unit>? = null
        var appsSnapshot = """{"users":[]}"""
        val entered = CompletableDeferred<Unit>()
        val calls = mutableListOf<ApiTestRequest>()

        override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
            calls += request
            if (request.path == "/android/apps") return apiSuccess(appsSnapshot)
            val data = when (request.method) {
                "GET" -> {
                    readGate?.await()
                    (readErrors[request.target] ?: readError)?.let { throw it }
                    val saved = contents[request.target] ?: when (request.target) {
                        LocalRuleSet.Policy.target -> defaultRoute
                        "singbox/outbounds" -> """{"outbounds":[{"type":"selector","tag":"AI"},{"type":"urltest","tag":"Auto"},{"type":"direct","tag":"direct"}]}"""
                        else -> content
                    }
                    JsonObject(mapOf("content" to JsonPrimitive(saved),
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

    private fun model(scope: CoroutineScope, transport: Transport): RoutingRulesViewModel {
        val client = ModuleApiClient(transport = transport)
        return RoutingRulesViewModel(ConfigRepository(client), AppPackageRepository(client), scope)
    }

    private fun RoutingRulesState.file(target: LocalRuleSet = LocalRuleSet.Proxy) = files.getValue(target)

    private suspend fun RoutingRulesViewModel.loaded(): RoutingRulesState = withTimeout(5_000) {
        state.first { !it.isLoading && it.files.values.all { file -> file.document != null || file.requiresReload } }
    }

    private suspend fun RoutingRulesViewModel.saved(): RoutingRulesState = withTimeout(5_000) {
        state.first { !it.isSaving }
    }

    @Test fun groupingKeepsDocumentAndIndexForApplicationAddressAndAdvancedRules() = runBlocking {
        val transport = Transport().apply {
            contents[LocalRuleSet.Proxy.target] = """{"version":1,"rules":[{"domain":"example.org"},{"domain":"example.net","port":443}]}"""
            contents[LocalRuleSet.Direct.target] = """{"version":1,"rules":[{"package_name":"com.android.chrome"}]}"""
            contents[LocalRuleSet.Block.target] = """{"version":1,"rules":[]}"""
            contents[LocalRuleSet.Policy.target] = """{"route":{"rules":[{"type":"default","package_name":["com.example.test"],"action":"route","outbound":"AI"}]}}"""
        }
        val vm = model(this, transport)
        vm.refresh()
        val state = vm.loaded()
        val grouped = state.groupedRules()
        assertEquals(listOf(RuleIdentity(LocalRuleSet.Direct, 0), RuleIdentity(LocalRuleSet.Policy, 0)),
            grouped.getValue(RuleSection.Apps).map { it.identity })
        assertEquals("AI", grouped.getValue(RuleSection.Apps).last().policyTag)
        assertEquals(RuleIdentity(LocalRuleSet.Proxy, 0), grouped.getValue(RuleSection.Address).single().identity)
        assertEquals(RuleIdentity(LocalRuleSet.Proxy, 1), grouped.getValue(RuleSection.Advanced).single().identity)
        assertFalse(transport.calls.any { it.mutation })
    }

    @Test fun ruleSheetDoesNotOpenAReusedIndexAfterRevisionChanges() = runBlocking {
        val identity = RuleIdentity(LocalRuleSet.Direct, 0)
        val transport = Transport().apply {
            contents[identity.target.target] = """{"version":1,"rules":[{"domain":"new.example"}]}"""
            revisions[identity.target.target] = "new-revision"
        }
        val vm = model(this, transport)
        vm.openEditor(identity, "old-revision")
        val state = vm.loaded()
        assertNull(state.draft)
        assertEquals("new-revision", state.file(identity.target).revision)
        vm.save()
        assertFalse(transport.calls.any { it.mutation })
    }

    @Test fun ruleSheetSavesMatchingRevisionAndCanOpenAnotherDraft() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        val identity = RuleIdentity(LocalRuleSet.Direct, 0)
        vm.openEditor(identity, transport.revision)
        val opened = withTimeout(5_000) { vm.state.first { it.draft != null } }
        assertEquals(identity.index, opened.draft?.index)
        assertEquals(identity.target, opened.draft?.target)
        vm.updateDraft(RuleField.DomainSuffix, "saved.example")
        vm.save()
        assertEquals(false, vm.saved().editor?.visible)
        assertEquals("saved.example", LocalRuleDocument.parse(transport.contents.getValue(identity.target.target))
            .rules.first().simpleRule()?.values?.single())
        vm.dismiss()
        vm.openEditor(identity, "saved-revision")
        val reopened = withTimeout(5_000) { vm.state.first { it.draft != null } }
        assertEquals("saved.example", reopened.draft?.text)
        assertEquals(1, transport.calls.count { it.mutation })
    }

    @Test fun closingRuleSheetDuringReadDoesNotCreateAHiddenDraft() = runBlocking {
        val gate = CompletableDeferred<Unit>()
        val transport = Transport().apply { readGate = gate }
        val vm = model(this, transport)
        vm.openEditor(null, null)
        assertTrue(vm.state.value.isLoading)
        vm.closeEditor()
        assertEquals(false, vm.state.value.editor?.visible)
        vm.dismiss()
        gate.complete(Unit)
        assertNull(vm.loaded().draft)
        assertNull(vm.state.value.editor)
        assertFalse(transport.calls.any { it.mutation })
    }

    @Test fun newRuleSheetCanRetryAfterReadFailure() = runBlocking {
        val transport = Transport().apply { readError = IllegalStateException("offline") }
        val vm = model(this, transport)
        vm.openEditor(null, null)
        assertNull(vm.loaded().draft)
        transport.readError = null
        vm.openEditor(null, null, discardDraft = true)
        val reopened = withTimeout(5_000) { vm.state.first { it.draft != null } }
        assertTrue(reopened.editor?.visible == true)
        assertFalse(transport.calls.any { it.mutation })
    }

    @Test fun applicationChoicesAreReadOnDemandAndDeduplicatedByPackage() = runBlocking {
        val transport = Transport().apply {
            appsSnapshot = """{"users":[
                {"id":10,"name":"Work","revision":"a","apps":[
                    {"package_name":"com.android.chrome","label":"Chrome (work)","uid":1010022,"system":false,"version_code":1,"last_update_time":1,"icon_key":"work"},
                    {"package_name":"com.example.profile","label":"","uid":1010023,"system":false,"version_code":1,"last_update_time":1,"icon_key":""}
                ]},
                {"id":0,"name":"Owner","revision":"b","apps":[
                    {"package_name":"com.android.chrome","label":"Chrome","uid":10022,"system":false,"version_code":1,"last_update_time":1,"icon_key":"owner"},
                    {"package_name":"com.google.android.gms","label":"Google Play services","uid":10023,"system":true,"version_code":1,"last_update_time":1,"icon_key":"gms"}
                ]}
            ]}"""
        }
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.add()
        vm.selectTarget(LocalRuleSet.Policy)
        vm.selectPolicy("AI")
        vm.updateDraft(RuleField.PackageName, "com.example.notinstalled")
        assertEquals(0, transport.calls.count { it.path == "/android/apps" })

        val apps = vm.installedApps()
        assertEquals(listOf("com.android.chrome", "com.example.profile", "com.google.android.gms"), apps.map { it.packageName })
        assertEquals("owner", apps.first().iconKey)
        assertEquals("com.example.profile", apps[1].label)
        assertEquals(apps, vm.installedApps())
        assertEquals(1, transport.calls.count { it.path == "/android/apps" })
        assertEquals("com.example.notinstalled", vm.state.value.draft?.text)

        vm.updateDraft(RuleField.PackageName, "com.example.notinstalled\n${apps.first().packageName}")
        assertTrue(transport.calls.none { it.mutation })
        vm.save()
        vm.saved()
        val saved = transport.contents.getValue(LocalRuleSet.Policy.target)
        val (destination, rule) = LocalRuleDocument.parse(saved, nativeRoute = true).rules
            .mapNotNull { it.policyRule() }.single()
        assertEquals("AI", destination)
        assertEquals(listOf("com.example.notinstalled", "com.android.chrome"), rule.values)
    }

    @Test fun initialLoadMergesAllGroupsWithDistinctFileAndIndexIdentities() = runBlocking {
        val transport = Transport()
        LocalRuleSet.shortcuts.forEach { target ->
            transport.contents[target.target] = """{"version":1,"rules":[{"domain":"${target.tag}.example"}]}"""
            transport.revisions[target.target] = "${target.tag}-revision"
        }
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        assertEquals(LocalRuleSet.entries.map { it.target } + "singbox/outbounds", transport.calls.map { it.target })
        assertEquals(LocalRuleSet.shortcuts.map { RuleIdentity(it, 0) }, loaded.rules)
        assertEquals(3, loaded.rules.distinct().size)
        assertTrue(loaded.hasDocuments)
        assertFalse(loaded.isEmpty)
        LocalRuleSet.shortcuts.forEach { target ->
            assertTrue(loaded.editable(target))
            assertEquals("${target.tag}-revision", loaded.file(target).revision)
            assertEquals(LocalRuleDocument.parse(transport.contents.getValue(target.target)), loaded.file(target).document)
        }
        vm.add()
        LocalRuleSet.entries.forEach(vm::selectTarget)
        vm.dismiss()
        assertEquals(5, transport.calls.size)
    }

    @Test fun newDraftSelectsAnyTargetWithoutReadingAndSavesOnlyThatFile() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        LocalRuleSet.shortcuts.forEach { target ->
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
        assertEquals(10, transport.calls.size)
    }

    @Test fun groupRuleUsesNativeRouteRevisionAndCanBeEditedAndDeleted() = runBlocking {
        val transport = Transport()
        transport.revisions[LocalRuleSet.Policy.target] = "route-revision"
        val vm = model(this, transport)
        vm.refresh()
        assertEquals(listOf("AI", "Auto"), vm.loaded().policyTargets)

        vm.add()
        vm.selectTarget(LocalRuleSet.Policy)
        vm.updateDraft(RuleField.DomainSuffix, "example.com")
        vm.selectPolicy("direct")
        vm.save()
        assertTrue(transport.calls.none { it.method == "PUT" })
        vm.selectPolicy("AI")
        vm.save()
        val identity = vm.saved().rules.single { it.target == LocalRuleSet.Policy }
        assertEquals("route-revision", transport.calls.last().text("revision"))
        assertEquals("AI", LocalRuleDocument.parse(transport.contents.getValue("singbox/route"), nativeRoute = true)
            .rules[identity.index].policyRule()?.first)

        vm.edit(identity)
        assertEquals("AI", vm.state.value.draft!!.policyTag)
        vm.selectPolicy("Auto")
        vm.save()
        assertEquals("Auto", vm.saved().file(LocalRuleSet.Policy).document!!.rules[identity.index].policyRule()?.first)
        vm.requestDelete(identity)
        vm.delete()
        assertTrue(vm.saved().rules.none { it.target == LocalRuleSet.Policy })
        assertEquals(LocalRuleDocument.parse(defaultRoute, nativeRoute = true),
            LocalRuleDocument.parse(transport.contents.getValue("singbox/route"), nativeRoute = true))
        assertTrue(transport.calls.filter { it.method == "PUT" }.all { it.target == "singbox/route" })
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
            assertEquals(5, transport.calls.count { it.method == "GET" })
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

    @Test fun cancelingDeleteReturnsToExistingDraftWithoutWriting() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        val identity = RuleIdentity(LocalRuleSet.Direct, 0)
        vm.edit(identity)
        vm.updateDraft(RuleField.DomainSuffix, "unsaved.example")
        vm.requestDelete(identity)
        assertEquals("unsaved.example", vm.state.value.draft?.text)
        assertEquals(identity, vm.state.value.deleting)
        vm.save()
        vm.cancelDelete()
        assertNull(vm.state.value.deleting)
        assertEquals("unsaved.example", vm.state.value.draft?.text)
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
