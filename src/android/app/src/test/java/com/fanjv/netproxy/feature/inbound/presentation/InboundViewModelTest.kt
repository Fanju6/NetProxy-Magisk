package com.fanjv.netproxy.feature.inbound.presentation

import com.fanjv.netproxy.core.command.*
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.feature.inbound.data.InboundRepository
import com.fanjv.netproxy.feature.inbound.data.textAt
import com.fanjv.netproxy.feature.inbound.data.listAt
import com.fanjv.netproxy.feature.inbound.data.objectAt
import com.fanjv.netproxy.feature.inbound.data.stringArray
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.CopyOnWriteArrayList

class InboundViewModelTest {
    @get:Rule val folder = TemporaryFolder()
    private val calls = CopyOnWriteArrayList<List<String>>()
    private var backend = "ebpf"
    private var rootPolicy = "default"
    private var running = true
    private var failure: String? = null
    private var readFailure: String? = null
    private var statusFailure: String? = null
    private var restartFailure: String? = null
    private var diagnosticFailure: String? = null
    private var onApply: () -> Unit = {}
    private var activeConfirmed = true
    private var actualBackendOverride: String? = null
    private var ebpf = """{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"dns_mode":"respect_policy","bypass_port":[853]},"shared":{"enabled":false},"udp_timeout":"3m"}"""
    private var tun = """{"type":"tun","tag":"netproxy-in","address":["172.19.0.1/30"],"auto_route":true,"auto_redirect":true,"exclude_interface":["old"],"route_address_set":["untouched"],"multi_queue":true}"""
    private var revision = 1
    private val partitionWrites = mutableMapOf<String, Int>()
    private fun configRevision(target: String) = "$target-$revision-${partitionWrites[target] ?: 0}"
    private var choicesGate: CountDownLatch? = null
    private val choicesEntered = CompletableDeferred<Unit>()
    private var choicesFailure = false

    private fun output(data: String) = NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
    private fun viewModel(scope: CoroutineScope, saveScope: CoroutineScope = scope,
        onSaveFailure: (String) -> Unit = {}): InboundViewModel {
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            calls += args
            when {
                args == listOf("service", "status") -> {
                    if (statusFailure != null) throw NetProxyCtlException(statusFailure!!, "status failed")
                    output("""{"state":"${if (running) "ready" else "stopped"}","configured_backend":"$backend","active_backend":${if (running && activeConfirmed) "\"${actualBackendOverride ?: backend}\"" else "null"}}""")
                }
                args == listOf("service", "restart") -> {
                    if (restartFailure != null) throw NetProxyCtlException(restartFailure!!, "restart failed")
                    output("{}")
                }
                args == listOf("ebpf", "status", "configured") -> {
                    val content = "结论: ${diagnosticFailure ?: "eBPF 能力预检通过"}"
                    val data = JsonObject(mapOf("content" to JsonPrimitive(content), "report" to JsonObject(emptyMap())))
                    if (diagnosticFailure != null) throw NetProxyCtlException("ebpf.unsupported", "预检未通过", data)
                    output(data.toString())
                }
                args.take(2) == listOf("config", "read") -> {
                    if (args.last() == "singbox/config.json") {
                        choicesGate?.let {
                            choicesEntered.complete(Unit)
                            check(it.await(5, TimeUnit.SECONDS))
                        }
                        if (choicesFailure) throw NetProxyCtlException("config.read_failed", "choices failed")
                    }
                    if (readFailure != null && args.last().startsWith("inbound/")) throw NetProxyCtlException(readFailure!!, "read failed")
                    val content = when (args.last()) {
                        "inbound/backend" -> """{"backend":"$backend"}"""
                        "inbound/root_policy" -> """{"root_policy":"$rootPolicy"}"""
                        "inbound/ebpf" -> """{"ebpf":$ebpf}"""
                        "inbound/tun" -> """{"tun":$tun}"""
                        "singbox/config.json" -> """{"route":{"rule_set":[{"tag":"private"}]}}"""
                        else -> error("非预期配置目标")
                    }
                    output(JsonObject(mapOf("content" to JsonPrimitive(content), "revision" to JsonPrimitive(configRevision(args.last())))).toString())
                }
                args.take(2) == listOf("config", "apply") -> {
                    onApply()
                    if (failure != null || args[3] != configRevision(args[4])) NetProxyCtlOutput(false,
                        listOf("""{"schema":1,"ok":false,"code":"${failure ?: "config.conflict"}","message":"failed"}"""), emptyList())
                    else {
                        val content = kotlinx.serialization.json.Json.parseToJsonElement(File(args.last()).readText()) as JsonObject
                        when (args[4]) {
                            "inbound/backend" -> backend = (content.getValue("backend") as JsonPrimitive).content
                            "inbound/root_policy" -> rootPolicy = (content.getValue("root_policy") as JsonPrimitive).content
                            "inbound/ebpf" -> ebpf = content.getValue("ebpf").toString()
                            "inbound/tun" -> tun = content.getValue("tun").toString()
                        }
                        partitionWrites[args[4]] = (partitionWrites[args[4]] ?: 0) + 1
                        output("""{"revision":"${configRevision(args[4])}"}""")
                    }
                }
                else -> error("非预期命令")
            }
        })
        val writes = ConfigurationWrites(saveScope) { onSaveFailure(it.message.orEmpty()) }
        return InboundViewModel(InboundRepository(ConfigRepository(client, CommandFileStore(folder.root),
            awaitPendingWrites = writes::await), ServiceRepository(client)), scope, writes)
    }

    private suspend fun InboundViewModel.loaded() = withTimeout(5_000) { state.first { it.snapshot != null && !it.isLoading } }
    private suspend fun InboundViewModel.idle() {
        flush()
        withTimeout(5_000) { state.first { !it.isSaving } }
    }

    @Test fun rootPolicyCommitsPendingParametersAndKeepsBackend() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        assertEquals("default", vm.state.value.snapshot!!.rootPolicy)
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        vm.setRootPolicy("exclude"); vm.idle()
        assertEquals(listOf("inbound/ebpf", "inbound/root_policy"),
            calls.filter { it.take(2) == listOf("config", "apply") }.map { it[4] })
        assertEquals("exclude", vm.state.value.snapshot!!.rootPolicy)
        assertEquals("ebpf", vm.state.value.backend)
        assertFalse(vm.state.value.hasPendingChanges)
        vm.setRootPolicy("exclude"); vm.idle()
        assertEquals(2, calls.count { it.take(2) == listOf("config", "apply") })
        running = false
        vm.requestBackend("tun"); vm.idle()
        assertEquals("tun", vm.state.value.backend)
        assertEquals("exclude", vm.state.value.snapshot!!.rootPolicy)
    }

    @Test fun rootPolicyFailureKeepsConfirmedValueAndReviewableCandidate() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        failure = "config.conflict"
        vm.setRootPolicy("include")
        withTimeout(5_000) { vm.state.first { it.requiresReload && !it.isSaving } }
        assertEquals("default", vm.state.value.snapshot!!.rootPolicy)
        assertEquals("config.conflict", vm.state.value.errorCode)
        val draft = vm.state.value.draftForReview()!!
        assertEquals("root_policy", draft.partition)
        assertEquals("include", kotlinx.serialization.json.Json.parseToJsonElement(draft.snapshot.content).jsonObject.textAt("root_policy"))
        vm.setRootPolicy("exclude")
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun confirmedRootPolicySurvivesPageScopeDisposal() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val vm = viewModel(pageScope, saveScope = this)
        try {
            vm.refresh(); vm.loaded()
            vm.setRootPolicy("include"); pageScope.cancel()
            withTimeout(5_000) { vm.state.first { !it.isSaving && it.snapshot!!.rootPolicy == "include" } }
            assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
        } finally { pageScope.cancel() }
    }

    @Test fun nativeCommitSurvivesPageScopeAndReportsFailure() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val errors = mutableListOf<String>()
        val vm = viewModel(pageScope, saveScope = this, onSaveFailure = errors::add)
        try {
            vm.refresh(); vm.loaded()
            vm.setField("udp_timeout", JsonPrimitive("5m"))
            failure = "config.conflict"
            vm.requestFlush(); pageScope.cancel()
            withTimeout(5_000) { vm.state.first { it.requiresReload && !it.isSaving } }
            assertEquals(listOf("failed"), errors)
            assertEquals("5m", vm.state.value.native.textAt("udp_timeout"))
            assertTrue(vm.state.value.hasPendingChanges)
            assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
            vm.requestFlush()
            assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
        } finally { pageScope.cancel() }
    }

    @Test fun nativeEditsMergeOnLeaveAndReturningToOriginalDoesNotWrite() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        vm.setField("local.dns_mode", JsonPrimitive("off"))
        vm.refresh()
        assertEquals("5m", vm.state.value.native.textAt("udp_timeout"))
        assertTrue(vm.state.value.hasPendingChanges)
        assertFalse(calls.any { it.take(2) == listOf("config", "apply") })
        assertTrue(vm.flush())
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
        vm.setField("udp_timeout", JsonPrimitive("3m"))
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        assertFalse(vm.state.value.hasPendingChanges)
        assertTrue(vm.flush())
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun confirmedBackendSwitchSurvivesPageDisposalAndReportsFailure() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val errors = mutableListOf<String>()
        val vm = viewModel(pageScope, saveScope = this, onSaveFailure = errors::add)
        try {
            vm.refresh(); vm.loaded()
            vm.requestBackend("tun")
            withTimeout(5_000) { vm.state.first { it.pendingBackend == "tun" } }
            failure = "tun.start_failed"
            vm.confirmBackendSwitch(); pageScope.cancel()
            withTimeout(5_000) { vm.state.first { it.requiresReload && !it.isSaving } }
            assertEquals(listOf("failed"), errors)
            assertEquals("ebpf", vm.state.value.backend)
            assertEquals("backend", vm.state.value.draftForReview()!!.partition)
            assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
        } finally { pageScope.cancel() }
    }

    @Test fun restartCompletesItsDraftAndConfirmationAfterPageDisposal() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val vm = viewModel(pageScope, saveScope = this)
        try {
            vm.refresh(); vm.loaded()
            vm.setField("udp_timeout", JsonPrimitive("5m"))
            vm.restart(); pageScope.cancel()
            withTimeout(5_000) { vm.state.first { !it.isSaving } }
            assertTrue(ebpf.contains("5m"))
            assertFalse(vm.state.value.hasPendingChanges)
            assertFalse(vm.state.value.requiresReload)
            assertEquals("ready", vm.state.value.snapshot!!.status!!.state)
            assertEquals(1, calls.count { it == listOf("service", "restart") })
        } finally { pageScope.cancel() }
    }

    @Test fun leavingBeforeBackendConfirmationNeverSwitchesRunningService() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val vm = viewModel(pageScope, saveScope = this)
        try {
            vm.refresh(); vm.loaded()
            vm.setField("udp_timeout", JsonPrimitive("5m"))
            vm.requestBackend("tun"); pageScope.cancel()
            withTimeout(5_000) { vm.state.first { !it.isSaving } }
            assertTrue(ebpf.contains("5m"))
            assertEquals("ebpf", backend)
            assertNull(vm.state.value.pendingBackend)
            assertFalse(calls.any { it.take(2) == listOf("config", "apply") && it[4] == "inbound/backend" })
        } finally { pageScope.cancel() }
    }

    @Test fun queuedSaveDoesNotIncludeEditsMadeAfterItsTrigger() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        vm.requestFlush()
        vm.setField("local.dns_mode", JsonPrimitive("off"))
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertTrue(ebpf.contains("respect_policy"))
        assertEquals("off", vm.state.value.native.objectAt("local").textAt("dns_mode"))
        assertTrue(vm.state.value.hasPendingChanges)
        assertTrue(vm.flush())
        assertTrue(ebpf.contains("off"))
        assertEquals(2, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun failedSaveAfterRevertingDraftStillBlocksNavigationAndAutomaticRefresh() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        val originalRevision = vm.state.value.snapshot!!.partitions.getValue("ebpf").revision
        failure = "config.conflict"
        onApply = { vm.setField("udp_timeout", JsonPrimitive("3m")) }
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        assertFalse(vm.flush())
        assertNull(vm.state.value.draft)
        assertFalse(vm.state.value.canReviewDraft)
        assertTrue(vm.state.value.requiresReload)
        val reads = calls.count { it.take(2) == listOf("config", "read") }
        vm.refresh()
        assertEquals(reads, calls.count { it.take(2) == listOf("config", "read") })
        assertEquals(originalRevision, vm.state.value.snapshot!!.partitions.getValue("ebpf").revision)
        failure = null
        vm.discardAndReload(); vm.loaded()
        assertFalse(vm.state.value.requiresReload)
    }

    @Test fun backendSwitchFlushesParametersBeforeRequestingConfirmation() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend == "tun" } }
        assertEquals("ebpf", vm.state.value.backend)
        assertTrue(ebpf.contains("5m"))
        assertFalse(vm.state.value.hasPendingChanges)
        vm.confirmBackendSwitch(); vm.idle()
        assertEquals(listOf("inbound/ebpf", "inbound/backend"),
            calls.filter { it.take(2) == listOf("config", "apply") }.map { it[4] })
        assertEquals("tun", vm.state.value.backend)
    }

    @Test fun failedFlushPreventsRestartAndRetainsLatestDraft() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        failure = "config.conflict"
        vm.restart(); vm.idle()
        assertFalse(vm.flush())
        assertTrue(vm.state.value.requiresReload)
        assertTrue(vm.state.value.draftForReview()!!.snapshot.content.contains("5m"))
        assertFalse(calls.any { it == listOf("service", "restart") })
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun revertingDuringSaveIsAppliedAfterConfirmationWithoutFlicker() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        onApply = {
            if (calls.count { it.take(2) == listOf("config", "apply") } == 1) {
                vm.setField("udp_timeout", JsonPrimitive("3m"))
                assertEquals("3m", vm.state.value.native.textAt("udp_timeout"))
            }
        }
        vm.setField("udp_timeout", JsonPrimitive("5m"))
        assertTrue(vm.flush())
        assertEquals("3m", vm.state.value.native.textAt("udp_timeout"))
        assertTrue(ebpf.contains("3m"))
        assertEquals(2, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun backgroundSaveKeepsLaterForegroundEditsUntilNextLeave() = runBlocking {
        val vm = viewModel(this)
        vm.refresh(); vm.loaded()
        onApply = {
            if (calls.count { it.take(2) == listOf("config", "apply") } == 1) {
                vm.setField("local.dns_mode", JsonPrimitive("off"))
            }
        }
        vm.setField("udp_timeout", JsonPrimitive("5m")); vm.requestFlush()
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
        assertTrue(ebpf.contains("respect_policy"))
        assertTrue(vm.state.value.hasPendingChanges)
        assertTrue(vm.flush())
        assertTrue(ebpf.contains("off"))
        assertEquals(2, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun diagnosticsInBothBackendsUseReadableContentWithoutChangingService() = runBlocking {
        for (selected in listOf("ebpf", "tun")) {
            backend = selected
            val vm = viewModel(this)
            vm.refresh()
            vm.loaded()
            calls.clear()
            vm.diagnose()
            withTimeout(5_000) { vm.state.first { it.diagnostic != null } }
            assertEquals("结论: eBPF 能力预检通过", vm.state.value.diagnostic)
            assertEquals(listOf(listOf("ebpf", "status", "configured")), calls)
            assertEquals(selected, vm.state.value.backend)
            assertFalse(vm.state.value.isDiagnosing)
            vm.dismissDiagnostic()
            assertNull(vm.state.value.diagnostic)
        }
    }

    @Test fun failedDiagnosticsShowTheReadableReportInsteadOfJsonOrGenericError() = runBlocking {
        backend = "tun"
        diagnosticFailure = "部分必要检查无法确认"
        val vm = viewModel(this)
        vm.diagnose()
        withTimeout(5_000) { vm.state.first { it.diagnostic != null } }
        assertEquals("结论: 部分必要检查无法确认", vm.state.value.diagnostic)
        assertFalse(vm.state.value.isDiagnosing)
        assertEquals(listOf(listOf("ebpf", "status", "configured")), calls)
    }

    @Test fun initialLoadPublishesTheFormAndChoicesTogether() = runBlocking {
        choicesGate = CountDownLatch(1)
        val vm = viewModel(this)
        assertTrue(vm.state.value.isInitialLoading)
        vm.refresh()
        withTimeout(5_000) { choicesEntered.await() }
        assertTrue(vm.state.value.isInitialLoading)
        assertNull(vm.state.value.snapshot)
        choicesGate!!.countDown()
        vm.loaded()
        assertFalse(vm.state.value.isInitialLoading)
        assertEquals(listOf("private"), vm.state.value.choices.ruleSets)
        assertTrue(vm.state.value.editable)
    }

    @Test fun refreshKeepsThePreviousFormUntilAllReadsFinish() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val original = vm.state.value.snapshot
        choicesGate = CountDownLatch(1)
        backend = "tun"
        vm.refresh()
        withTimeout(5_000) { choicesEntered.await() }
        assertFalse(vm.state.value.isInitialLoading)
        assertEquals(original, vm.state.value.snapshot)
        assertTrue(vm.state.value.editable)
        choicesGate!!.countDown()
        vm.loaded()
        assertEquals("tun", vm.state.value.backend)
        assertTrue(vm.state.value.editable)
    }

    @Test fun savingCancelsAnOlderRefreshWithoutReplacingTheNewRevision() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        choicesGate = CountDownLatch(1)
        vm.refresh()
        withTimeout(5_000) { choicesEntered.await() }
        assertTrue(vm.state.value.hasConfiguration)
        val refresh = coroutineContext[Job]!!.children.single()
        vm.setField("local.dns_mode", JsonPrimitive("off"))
        vm.idle()
        val saved = vm.state.value.snapshot
        assertFalse(vm.state.value.isLoading)
        assertEquals("off", vm.state.value.native.getValue("local").jsonObject.textAt("dns_mode"))
        choicesGate!!.countDown()
        withTimeout(5_000) { refresh.join() }
        assertEquals(saved, vm.state.value.snapshot)
    }

    @Test fun applyingKeepsNewDraftVisibleAndConfirmationBlocksBackendEdits() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        onApply = {
            assertTrue(vm.state.value.isSaving)
            assertEquals("ebpf", vm.state.value.applyingField)
            assertTrue(vm.state.value.hasConfiguration)
            assertTrue(vm.state.value.editable)
            if (calls.count { it.take(2) == listOf("config", "apply") } == 1) {
                vm.setField("local.dns_mode", JsonPrimitive("hijack"))
            }
        }
        vm.setField("local.dns_mode", JsonPrimitive("off"))
        vm.idle()
        assertNull(vm.state.value.applyingField)
        assertEquals(2, calls.count { it.take(2) == listOf("config", "apply") })
        assertEquals("hijack", vm.state.value.native.getValue("local").jsonObject.textAt("dns_mode"))
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend != null } }
        assertNull(vm.state.value.applyingField)
        assertTrue(vm.state.value.hasConfiguration)
        assertFalse(vm.state.value.editable)
        vm.cancelBackendSwitch()
        assertTrue(vm.state.value.editable)
    }

    @Test fun choicesFailureDoesNotPreventDisplayingLoadedConfiguration() = runBlocking {
        choicesFailure = true
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        assertFalse(vm.state.value.isInitialLoading)
        assertTrue(vm.state.value.choicesError)
        assertTrue(vm.state.value.editable)
    }

    @Test fun firstLoadFailureShowsRetryRatherThanAnEndlessSpinner() = runBlocking {
        readFailure = "config.read_failed"
        val vm = viewModel(this)
        vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.isLoading } }
        assertNull(vm.state.value.snapshot)
        assertFalse(vm.state.value.isInitialLoading)
        assertTrue(vm.state.value.requiresReload)
        assertFalse(vm.state.value.editable)
        readFailure = null
        vm.refresh()
        vm.loaded()
        assertTrue(vm.state.value.editable)
    }

    @Test fun runningSwitchDoesNotChangeSelectionUntilConfirmed() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend == "tun" } }
        assertEquals("ebpf", vm.state.value.backend)
        assertFalse(calls.any { it.take(2) == listOf("config", "apply") })
        vm.cancelBackendSwitch()
        assertNull(vm.state.value.pendingBackend)
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend == "tun" } }
        vm.confirmBackendSwitch()
        vm.idle()
        assertEquals("tun", vm.state.value.backend)
        assertEquals("tun", vm.state.value.snapshot!!.status!!.activeBackend)
    }

    @Test fun failedSwitchRestoresSelectionAndRequiresReload() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        failure = "tun.start_failed"
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend != null } }
        vm.confirmBackendSwitch()
        vm.idle()
        assertEquals("ebpf", vm.state.value.backend)
        assertTrue(vm.state.value.requiresReload)
        assertFalse(vm.state.value.editable)
        assertEquals("failed", vm.state.value.error)
        assertNull(vm.state.value.applyingField)
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun conflictDoesNotOverwriteOrRetryAndReloadUnblocks() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val oldContent = ebpf
        failure = "config.conflict"
        vm.setField("local.dns_mode", JsonPrimitive("off"))
        vm.idle()
        assertEquals(oldContent, ebpf)
        assertTrue(vm.state.value.requiresReload)
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
        vm.setField("local.dns_mode", JsonPrimitive("hijack"))
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
        failure = null
        vm.discardAndReload()
        withTimeout(5_000) { vm.state.first { !it.isLoading && !it.requiresReload } }
        vm.setField("local.dns_mode", JsonPrimitive("off"))
        vm.idle()
        assertFalse(vm.state.value.requiresReload)
        val native = kotlinx.serialization.json.Json.parseToJsonElement(ebpf) as JsonObject
        assertEquals("3m", native.textAt("udp_timeout"))
        assertEquals("off", (native.getValue("local") as JsonObject).textAt("dns_mode"))
        assertEquals("""[853]""", (native.getValue("local") as JsonObject)["bypass_port"].toString())
    }

    @Test fun stoppedSwitchPreservesStoppedStateAndOnlyChangesBackendPartition() = runBlocking {
        running = false
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val previous = ebpf
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.backend == "tun" && !it.isSaving } }
        assertNull(vm.state.value.pendingBackend)
        assertEquals("stopped", vm.state.value.snapshot!!.status!!.state)
        assertNull(vm.state.value.snapshot!!.status!!.activeBackend)
        assertEquals(previous, ebpf)
        assertTrue(calls.filter { it.first() == "service" }.all { it == listOf("service", "status") })
    }

    @Test fun absentActiveBackendIsAnErrorNotSavedSuccess() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend != null } }
        activeConfirmed = false
        vm.confirmBackendSwitch()
        vm.idle()
        assertTrue(vm.state.value.requiresReload)
        assertTrue(vm.state.value.error.isNotBlank())
        assertNull(vm.state.value.snapshot!!.status!!.activeBackend)
    }

    @Test fun fieldDraftCannotBorrowNewRevisionAfterRefresh() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val draftRevision = vm.state.value.snapshot!!.partitions.getValue("ebpf").revision
        revision++
        ebpf = ebpf.replace("respect_policy", "hijack")
        vm.refresh()
        vm.loaded()
        vm.setField("local.dns_mode", JsonPrimitive("off"), "ebpf", draftRevision)
        vm.idle()
        assertTrue(vm.state.value.requiresReload)
        assertEquals("config.conflict", vm.state.value.errorCode)
        assertTrue(ebpf.contains("hijack"))
        assertFalse(ebpf.contains("off"))
    }

    @Test fun draftForPreviousBackendCannotModifyCurrentBackend() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val draftRevision = vm.state.value.snapshot!!.partitions.getValue("ebpf").revision
        backend = "tun"
        vm.refresh()
        vm.loaded()
        vm.setField("local.dns_mode", JsonPrimitive("off"), "ebpf", draftRevision)
        assertTrue(vm.state.value.requiresReload)
        assertEquals("config.conflict", vm.state.value.errorCode)
        assertFalse(calls.any { it.take(2) == listOf("config", "apply") })
    }

    @Test fun applyingConfiguredBackendStillConfirmsWhenActualDiffers() = runBlocking {
        backend = "tun"
        actualBackendOverride = "ebpf"
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend == "tun" } }
        assertFalse(calls.any { it.take(2) == listOf("config", "apply") })
        actualBackendOverride = null
        vm.confirmBackendSwitch()
        vm.idle()
        assertEquals("tun", vm.state.value.snapshot!!.status!!.activeBackend)
        assertFalse(vm.state.value.requiresReload)
    }

    @Test fun failedSwitchWithUnreadableConfigMarksActualUnknownAndKeepsDraftRevision() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val originalRevision = vm.state.value.snapshot!!.partitions.getValue("backend").revision
        failure = "tun.start_failed"
        onApply = { running = false; readFailure = "config.read_failed" }
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend != null } }
        vm.confirmBackendSwitch()
        vm.idle()

        assertNull(vm.state.value.snapshot!!.status)
        assertFalse(vm.state.value.editable)
        assertTrue(vm.state.value.requiresReload)
        assertEquals("tun.start_failed", vm.state.value.errorCode)
        val draft = vm.state.value.draftForReview()!!
        assertEquals("backend", draft.partition)
        assertEquals(originalRevision, draft.snapshot.revision)
        assertEquals("tun", kotlinx.serialization.json.Json.parseToJsonElement(draft.snapshot.content).jsonObject.textAt("backend"))

        vm.refresh()
        vm.loaded()
        assertNull(vm.state.value.snapshot!!.status)
        assertEquals(draft, vm.state.value.failedDraft)
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })

        readFailure = null
        revision++
        vm.discardAndReload()
        vm.loaded()
        assertEquals("stopped", vm.state.value.snapshot!!.status!!.state)
        assertNull(vm.state.value.snapshot!!.status!!.activeBackend)
        assertFalse(vm.state.value.requiresReload)
        assertNull(vm.state.value.failedDraft)
        assertNotEquals(originalRevision, vm.state.value.snapshot!!.partitions.getValue("backend").revision)
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun failedFieldSaveWithUnreadableStatusKeepsCandidateUntilExplicitDiscard() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val original = ebpf
        val originalRevision = vm.state.value.snapshot!!.partitions.getValue("ebpf").revision
        failure = "inbound.reload_failed"
        onApply = { running = false; statusFailure = "service.status_failed" }
        vm.setField("local.dns_mode", JsonPrimitive("off"))
        vm.idle()

        assertNull(vm.state.value.snapshot!!.status)
        assertFalse(vm.state.value.editable)
        assertTrue(vm.state.value.requiresReload)
        assertEquals(original, ebpf)
        val draft = vm.state.value.draftForReview()!!
        assertEquals("ebpf", draft.partition)
        assertEquals(originalRevision, draft.snapshot.revision)
        val native = kotlinx.serialization.json.Json.parseToJsonElement(draft.snapshot.content).jsonObject.getValue("ebpf").jsonObject
        assertEquals("off", native.getValue("local").jsonObject.textAt("dns_mode"))
        assertEquals("3m", native.textAt("udp_timeout"))

        vm.discardDraft()
        assertNull(vm.state.value.failedDraft)
        assertNull(vm.state.value.snapshot!!.status)
        assertTrue(vm.state.value.requiresReload)
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun successfulNativeWriteWithoutStatusConfirmationIsNotUiSuccess() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.pendingBackend != null } }
        onApply = { statusFailure = "service.status_failed" }
        vm.confirmBackendSwitch()
        vm.idle()

        assertEquals("tun", backend)
        assertEquals("ebpf", vm.state.value.backend)
        assertNull(vm.state.value.snapshot!!.status)
        assertEquals("service.status_failed", vm.state.value.errorCode)
        assertTrue(vm.state.value.requiresReload)
        assertNotNull(vm.state.value.failedDraft)
        assertFalse(vm.state.value.editable)

        statusFailure = null
        vm.discardAndReload()
        vm.loaded()
        assertEquals("tun", vm.state.value.backend)
        assertEquals("tun", vm.state.value.snapshot!!.status!!.activeBackend)
        assertNull(vm.state.value.failedDraft)
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun manualRefreshFailureDoesNotReuseReadySnapshot() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        assertEquals("ready", vm.state.value.snapshot!!.status!!.state)
        readFailure = "config.read_failed"
        vm.refresh()
        vm.loaded()
        assertNull(vm.state.value.snapshot!!.status)
        assertTrue(vm.state.value.requiresReload)
        assertFalse(vm.state.value.editable)
        assertFalse(calls.any { it.take(2) == listOf("config", "apply") })
    }

    @Test fun backendStatusReadFailureMarksActualUnknownBeforeAnyWrite() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        statusFailure = "service.status_failed"
        vm.requestBackend("tun")
        vm.idle()
        assertNull(vm.state.value.snapshot!!.status)
        assertTrue(vm.state.value.requiresReload)
        assertFalse(vm.state.value.editable)
        assertNull(vm.state.value.pendingBackend)
        assertFalse(calls.any { it.take(2) == listOf("config", "apply") })
    }

    @Test fun failedRestartDoesNotReuseReadySnapshot() = runBlocking {
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        restartFailure = "service.start_failed"
        vm.restart()
        vm.idle()
        assertNull(vm.state.value.snapshot!!.status)
        assertTrue(vm.state.value.requiresReload)
        assertFalse(vm.state.value.editable)
        assertNull(vm.state.value.failedDraft)
        assertEquals("restart failed", vm.state.value.error)
        assertEquals(1, calls.count { it == listOf("service", "restart") })
    }

    @Test fun tunInterfaceAndRuleSetEditsWriteWholeArrayTokensAndPreserveOtherFields() = runBlocking {
        backend = "tun"
        running = false
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        val interfaces = InboundListInput(listOf("ap,0", "usb0")).withSelection("manual0", true).entries()
        onApply = { assertEquals("tun", vm.state.value.applyingField) }
        vm.setFilter("include_interface", "exclude_interface", "include", interfaces)
        val selected = vm.state.value.native
        assertEquals(interfaces, selected.listAt("include_interface"))
        assertFalse(selected.containsKey("exclude_interface"))
        assertEquals(listOf("untouched"), selected.listAt("route_address_set"))

        val tags = InboundListInput(emptyList()).withText("corp,lan\nprivate").withSelection("more，rules", true).entries()
        vm.setField("route_exclude_address_set", stringArray(tags))
        vm.idle()
        val updated = kotlinx.serialization.json.Json.parseToJsonElement(tun).jsonObject
        assertEquals(tags, updated.listAt("route_exclude_address_set"))
        assertEquals(selected.getValue("include_interface"), updated.getValue("include_interface"))
        assertEquals(selected.getValue("multi_queue"), updated.getValue("multi_queue"))
        assertEquals(selected.getValue("route_address_set"), updated.getValue("route_address_set"))
        assertEquals("stopped", vm.state.value.snapshot!!.status!!.state)
        assertEquals(1, calls.count { it.take(2) == listOf("config", "apply") })
    }

    @Test fun multiFieldEditsTrackTheirVisibleSettingAndClearProgress() = runBlocking {
        running = false
        val vm = viewModel(this)
        vm.refresh()
        vm.loaded()
        onApply = { assertEquals("ebpf", vm.state.value.applyingField) }
        vm.setDataPaths("both")
        vm.idle()
        assertNull(vm.state.value.applyingField)

        vm.refresh()
        vm.loaded()
        onApply = { assertEquals("backend", vm.state.value.applyingField) }
        vm.requestBackend("tun")
        withTimeout(5_000) { vm.state.first { it.backend == "tun" && !it.isSaving } }
        assertNull(vm.state.value.applyingField)

        vm.refresh()
        vm.loaded()
        onApply = { assertEquals("tun", vm.state.value.applyingField) }
        vm.setTunIpv6(true)
        vm.idle()
        assertNull(vm.state.value.applyingField)
        assertTrue(vm.state.value.native.listAt("address").any { ':' in it })
    }
}
