package com.fanjv.netproxy.feature.routing

import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import com.fanjv.netproxy.feature.routing.model.LocalRuleDocument
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
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

    private class Transport : NetProxyCtlTransport {
        var content = """{"version":1,"rules":[{"domain_suffix":"example.com"},{"domain":"advanced.example","port":443}]}"""
        var revision = "read-revision"
        var applyError: NetProxyCtlException? = null
        var readError: Exception? = null
        val contents = mutableMapOf<String, String>()
        val revisions = mutableMapOf<String, String>()
        val readErrors = mutableMapOf<String, Exception>()
        var gate: CountDownLatch? = null
        val entered = CompletableDeferred<Unit>()
        val calls = mutableListOf<List<String>>()

        override suspend fun execute(arguments: List<String>, timeoutMillis: Long): NetProxyCtlOutput {
            calls += arguments
            val data = when (arguments[1]) {
                "read" -> {
                    (readErrors[arguments[2]] ?: readError)?.let { throw it }
                    JsonObject(mapOf("content" to JsonPrimitive(contents[arguments[2]] ?: content),
                        "revision" to JsonPrimitive(revisions[arguments[2]] ?: revision)))
                }
                "apply" -> {
                    gate?.let { entered.complete(Unit); check(it.await(5, TimeUnit.SECONDS)) }
                    applyError?.let { throw it }
                    val target = arguments[4]
                    assertEquals(listOf("config", "apply", "--revision", revisions[target] ?: revision), arguments.take(4))
                    contents[target] = File(arguments.last()).readText()
                    revisions[target] = "saved-revision"
                    JsonObject(mapOf("revision" to JsonPrimitive("saved-revision")))
                }
                else -> error("Unexpected command")
            }
            return NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"config.test","message":"","data":$data}"""), emptyList())
        }
    }

    private fun model(scope: CoroutineScope, transport: Transport) = RoutingRulesViewModel(
        ConfigRepository(NetProxyCtlClient(transport = transport), CommandFileStore(folder.root)), scope)

    private suspend fun RoutingRulesViewModel.loaded(): RoutingRulesState = withTimeout(5_000) {
        state.first { !it.isLoading && it.document != null }
    }

    private suspend fun RoutingRulesViewModel.saved(): RoutingRulesState = withTimeout(5_000) {
        state.first { !it.isSaving }
    }

    @Test fun savesOnlySelectedFileWithRevisionAndPreservesAdvancedRule() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        val advanced = vm.state.value.document!!.rules[1]
        vm.select(LocalRuleSet.Direct)
        vm.loaded()
        vm.edit()
        vm.updateDraft(RuleField.Port, "443\n8443")
        vm.save()
        val saved = vm.saved()
        assertNull(saved.draft)
        assertEquals("saved-revision", saved.revision)
        val document = saved.document!!
        assertEquals(3, document.rules.size)
        assertEquals(advanced, document.rules[1])
        assertEquals("singbox/rules/local/direct.json", transport.calls.last()[4])
        assertFalse(transport.calls.any { it.first() != "config" })
        vm.requestDelete(0)
        vm.delete()
        assertEquals(2, vm.saved().document!!.rules.size)
        assertEquals(advanced, LocalRuleDocument.parse(transport.contents.getValue(LocalRuleSet.Direct.target)).rules[0])
    }

    @Test fun initialLoadReadsAllGroupsOnceAndSwitchingDoesNotLoad() = runBlocking {
        val transport = Transport()
        LocalRuleSet.entries.forEach { selected ->
            transport.contents[selected.target] = """{"version":1,"rules":[{"domain":"${selected.tag}.example"}]}"""
            transport.revisions[selected.target] = "${selected.tag}-revision"
        }
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        assertEquals(LocalRuleSet.entries.map { listOf("config", "read", it.target) }, transport.calls)
        repeat(3) {
            LocalRuleSet.entries.forEach { selected ->
                vm.select(selected)
                assertFalse(vm.state.value.isLoading)
                assertEquals("${selected.tag}-revision", vm.state.value.revision)
                assertEquals(LocalRuleDocument.parse(transport.contents.getValue(selected.target)), vm.state.value.document)
            }
        }
        assertEquals(3, transport.calls.size)
    }

    @Test fun savedGroupRetainsLatestRevisionAndOtherGroupsWhenSwitching() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val original = vm.loaded().document
        vm.select(LocalRuleSet.Direct)
        vm.edit()
        vm.updateDraft(RuleField.Domain, "new.example")
        vm.save()
        val saved = vm.saved()
        val calls = transport.calls.size
        vm.select(LocalRuleSet.Proxy)
        assertEquals(original, vm.state.value.document)
        assertEquals("read-revision", vm.state.value.revision)
        vm.select(LocalRuleSet.Direct)
        assertEquals(saved.document, vm.state.value.document)
        assertEquals("saved-revision", vm.state.value.revision)
        assertEquals(calls, transport.calls.size)
    }

    @Test fun failedGroupDoesNotPreventOtherGroupsFromLoadingAndRefreshRecovers() = runBlocking {
        val transport = Transport()
        transport.readErrors[LocalRuleSet.Direct.target] = NetProxyCtlException("config.read_failed", "direct read failed")
        transport.contents[LocalRuleSet.Block.target] = "{}"
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        assertTrue(vm.state.value.editable)
        vm.select(LocalRuleSet.Direct)
        assertEquals("direct read failed", vm.state.value.error)
        assertTrue(vm.state.value.requiresReload)
        assertNull(vm.state.value.document)
        assertFalse(vm.state.value.editable)
        vm.select(LocalRuleSet.Block)
        assertTrue(vm.state.value.invalidDocument)
        assertFalse(vm.state.value.editable)
        transport.readErrors.clear()
        transport.contents.clear()
        vm.refresh()
        vm.loaded()
        LocalRuleSet.entries.forEach { selected ->
            vm.select(selected)
            assertTrue(vm.state.value.editable)
            assertFalse(vm.state.value.invalidDocument)
            assertEquals("", vm.state.value.error)
        }
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
        vm.loaded()
        vm.select(LocalRuleSet.Block)
        assertEquals("external-revision", vm.state.value.revision)
        assertEquals(LocalRuleDocument.parse(external), vm.state.value.document)
        assertEquals(6, transport.calls.size)
    }

    @Test fun conflictRetainsDraftAndCannotOverwriteNewRevision() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.edit(0)
        vm.updateDraft(RuleField.DomainSuffix, "new.example")
        transport.applyError = NetProxyCtlException("config.conflict", "changed")
        transport.revision = "external-revision"
        vm.save()
        val failed = vm.saved()
        assertTrue(failed.requiresReload)
        assertEquals("read-revision", failed.revision)
        assertEquals("new.example", failed.draft!!.text)
        val calls = transport.calls.size
        vm.save()
        vm.refresh()
        assertEquals(calls, transport.calls.size)
        vm.refresh(discardDraft = true)
        assertEquals("external-revision", vm.loaded().revision)
        assertNull(vm.state.value.draft)
    }

    @Test fun validationFailureKeepsDraftAndAllowsCorrectionOnlyWithUnchangedRevision() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.edit()
        vm.updateDraft(RuleField.DomainRegex, "[")
        transport.applyError = NetProxyCtlException("command.failed", "invalid regex")
        vm.save()
        val failed = vm.saved()
        assertFalse(failed.requiresReload)
        assertEquals("invalid regex", failed.error)
        assertEquals("[", failed.draft!!.text)
        transport.applyError = null
        vm.updateDraft(RuleField.DomainRegex, "^example\\.com$")
        vm.save()
        assertNull(vm.saved().draft)
    }

    @Test fun inFlightSaveBlocksRefreshCategoryChangeDuplicateWritesAndDismissal() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        transport.gate = CountDownLatch(1)
        vm.edit()
        vm.updateDraft(RuleField.Domain, "new.example")
        vm.save()
        withTimeout(5_000) { transport.entered.await() }
        vm.refresh()
        vm.select(LocalRuleSet.Block)
        vm.dismiss()
        vm.save()
        assertEquals(LocalRuleSet.Proxy, vm.state.value.selected)
        assertNotNull(vm.state.value.draft)
        assertEquals(1, transport.calls.count { it[1] == "apply" })
        transport.gate!!.countDown()
        assertEquals("saved-revision", vm.saved().revision)
    }

    @Test fun rawEditorReturnRefreshesAndMalformedDocumentIsNotReplaced() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.edit(1)
        assertNull(vm.state.value.draft!!.field)
        vm.save()
        assertFalse(transport.calls.any { it[1] == "apply" })
        vm.dismiss()
        transport.content = """{"version":5,"rules":[{"ip_cidr":"192.0.2.0/24"}]}"""
        transport.revision = "raw-editor-revision"
        vm.refresh()
        assertEquals("raw-editor-revision", vm.loaded().revision)
        transport.content = "{}"
        vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.isLoading && it.invalidDocument } }
        assertNull(vm.state.value.document)
        vm.edit()
        assertNull(vm.state.value.draft)
        assertFalse(transport.calls.any { it[1] == "apply" })
    }

    @Test fun deleteFromEditorClosesDraftAndCancellationDoesNotWrite() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        vm.loaded()
        vm.edit(0)
        vm.updateDraft(RuleField.DomainSuffix, "unsaved.example")
        vm.requestDelete(0)
        assertNull(vm.state.value.draft)
        assertEquals(0, vm.state.value.deleting)
        vm.dismiss()
        assertNull(vm.state.value.deleting)
        assertEquals("example.com", (vm.state.value.document!!.rules[0] as JsonObject)["domain_suffix"]?.let { (it as JsonPrimitive).content })
        assertFalse(transport.calls.any { it[1] == "apply" })
    }

    @Test fun failedRefreshRetainsDisplayedDataButDisablesSavingUntilReload() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.refresh()
        val loaded = vm.loaded()
        transport.readError = NetProxyCtlException("config.read_failed", "read failed")
        vm.refresh()
        val failed = withTimeout(5_000) { vm.state.first { !it.isLoading && it.requiresReload } }
        assertEquals(loaded.document, failed.document)
        assertFalse(failed.editable)
        assertEquals("read failed", failed.error)
    }
}
