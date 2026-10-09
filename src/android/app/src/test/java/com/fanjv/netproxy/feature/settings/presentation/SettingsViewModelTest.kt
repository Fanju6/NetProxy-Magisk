package com.fanjv.netproxy.feature.settings.presentation

import com.fanjv.netproxy.core.command.*
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.settings.model.ModuleAutoStartConfig
import com.fanjv.netproxy.feature.settings.model.ModuleWifiConfig
import com.fanjv.netproxy.feature.settings.model.WifiPolicySettings
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList

class SettingsViewModelTest {
    @get:Rule val folder = TemporaryFolder()

    private inner class Network {
        var wifi = WifiPolicySettings()
        var autoStart = true
        var wifiRevision = 0
        var autoStartRevision = 10
        val calls = CopyOnWriteArrayList<List<String>>()
        val writes get() = calls.count { it[1] == "apply" }
        val reads get() = calls.count { it[1] == "read" }
        var before: suspend (String) -> Unit = {}
        var beforeRead: suspend (String) -> Unit = {}

        fun viewModel(scope: CoroutineScope, writes: ConfigurationWrites = ConfigurationWrites(scope)): SettingsViewModel {
            val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
                calls += args
                val target = if (args[1] == "read") args[2] else args[4]
                assertTrue(target in setOf("module/wifi", "module/auto_start"))
                val data = if (args[1] == "read") {
                    beforeRead(target)
                    val content = if (target == "module/wifi") Json.encodeToString(ModuleWifiConfig(wifi))
                        else Json.encodeToString(ModuleAutoStartConfig(autoStart))
                    val revision = if (target == "module/wifi") "wifi-$wifiRevision" else "auto-$autoStartRevision"
                    JsonObject(mapOf("content" to JsonPrimitive(content), "revision" to JsonPrimitive(revision))).toString()
                } else {
                    assertEquals(listOf("config", "apply", "--revision"), args.take(3))
                    before(target)
                    val content = File(args.last()).readText()
                    assertEquals(setOf(target.substringAfter('/')), Json.parseToJsonElement(content).jsonObject.keys)
                    val revision = if (target == "module/wifi") {
                        if (args[3] != "wifi-$wifiRevision") throw NetProxyCtlException("config.conflict", "conflict")
                        wifi = Json.decodeFromString<ModuleWifiConfig>(content).wifi
                        "wifi-${++wifiRevision}"
                    } else {
                        if (args[3] != "auto-$autoStartRevision") throw NetProxyCtlException("config.conflict", "conflict")
                        autoStart = Json.decodeFromString<ModuleAutoStartConfig>(content).enabled
                        "auto-${++autoStartRevision}"
                    }
                    """{"revision":"$revision"}"""
                }
                NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
            })
            return SettingsViewModel(ConfigRepository(client, CommandFileStore(folder.root), writes::await), scope, writes)
        }
    }

    private suspend fun SettingsViewModel.loaded() = withTimeout(5_000) { state.first { it.hasLoaded && !it.isLoading } }
    private suspend fun SettingsViewModel.saved() = withTimeout(5_000) { state.first { !it.isSaving } }

    private suspend fun SettingsViewModel.flushWifi(): Boolean = withTimeout(5_000) {
        do {
            requestWifiFlush()
            state.first { !it.isSavingWifi }
        } while (state.value.hasPendingWifi && !state.value.requiresReload)
        !state.value.requiresReload && !state.value.hasPendingWifi
    }

    @Test fun wifiSelectionDerivesFromSwitchWithoutChangingSavedMode() = runBlocking {
        for (enabled in listOf(false, true)) {
            val network = Network()
            network.wifi = WifiPolicySettings(enabled = enabled, mode = "whitelist")
            val vm = network.viewModel(this)
            vm.refresh(); vm.loaded()
            assertEquals(enabled, vm.state.value.wifi.enabled)
            assertEquals(if (enabled) "whitelist" else "off", vm.state.value.wifi.selection)
            assertEquals("whitelist", vm.state.value.wifi.mode)
            assertFalse(vm.state.value.hasPendingWifi)
            assertEquals(0, network.writes)
        }
    }

    @Test fun confirmedWifiEditsMergeOnlyOnLeaveAndNoOpDoesNotWrite() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiSsidMode("blacklist")
        vm.setWifiSsidMode("whitelist")
        vm.setWifiSsids(listOf("home", "second", "home"))
        vm.setProxyOnNonWifi(false)
        assertEquals(0, network.writes)
        assertTrue(vm.state.value.hasPendingWifi)
        assertTrue(vm.flushWifi())
        assertEquals(1, network.writes)
        assertEquals(listOf("home", "second"), network.wifi.whitelist)
        assertTrue(network.wifi.enabled)
        assertEquals("whitelist", network.wifi.mode)
        assertFalse(network.wifi.proxyOnNonWifi)
        assertTrue(network.autoStart)
        assertEquals(2, network.reads)
        vm.setWifiSsidMode("off"); vm.setWifiSsidMode("whitelist")
        assertFalse(vm.state.value.hasPendingWifi)
        assertTrue(vm.flushWifi())
        assertEquals(1, network.writes)
    }

    @Test fun dirtyResumeAndConflictNeverBorrowNewRevisionOrLoseDraft() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiSsids(listOf("draft"))
        network.wifi = network.wifi.copy(blacklist = listOf("remote"))
        network.wifiRevision++
        network.autoStart = false
        network.autoStartRevision++
        vm.refresh()
        assertEquals(2, network.reads)
        assertFalse(vm.flushWifi())
        assertEquals(listOf("draft"), vm.state.value.wifi.ssids)
        assertEquals(listOf("remote"), network.wifi.blacklist)
        assertFalse(vm.flushWifi())
        assertEquals(1, network.writes)
        vm.discardWifiAndReload(); vm.loaded()
        assertFalse(vm.state.value.hasPendingWifi)
        assertEquals(listOf("remote"), vm.state.value.wifi.ssids)
        assertFalse(vm.state.value.autoStartEnabled)
    }

    @Test fun wifiListsRemainIndependentAndKeepExactNames() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        val blacklist = listOf(" Home,Wi-Fi ", "办公\"网络", "Cafe\\Guest")
        val whitelist = listOf("Office", "office")
        vm.setWifiSsidMode("blacklist"); vm.setWifiSsids(blacklist)
        vm.setWifiSsidMode("whitelist"); vm.setWifiSsids(whitelist)
        vm.setWifiSsidMode("off")
        assertTrue(vm.flushWifi())
        vm.refresh(); vm.loaded()
        assertEquals(blacklist, vm.state.value.wifi.blacklist)
        assertEquals(whitelist, vm.state.value.wifi.whitelist)
        assertFalse(vm.state.value.wifi.enabled)
        assertEquals("off", vm.state.value.wifi.selection)
        assertEquals("whitelist", network.wifi.mode)
        vm.setWifiSsidMode("blacklist")
        assertEquals(blacklist, vm.state.value.wifi.ssids)
        vm.setWifiSsidMode("whitelist")
        assertEquals(whitelist, vm.state.value.wifi.ssids)
    }

    @Test fun queuedSaveOnlyIncludesWifiValuesAtItsTrigger() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiSsidMode("blacklist"); vm.requestWifiFlush()
        vm.setWifiSsids(listOf("later"))
        vm.saved()
        assertTrue(network.wifi.blacklist.isEmpty())
        assertEquals(listOf("later"), vm.state.value.wifi.ssids)
        assertTrue(vm.state.value.hasPendingWifi)
        assertTrue(vm.flushWifi())
        assertEquals(listOf("later"), network.wifi.blacklist)
        assertEquals(2, network.writes)
    }

    @Test fun cancellingLeaveDoesNotCancelBackgroundSaveOrOverwriteLaterEdits() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        network.before = { if (network.writes == 1) {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        vm.setWifiSsidMode("blacklist")
        val leaving = launch { vm.flushWifi() }
        withTimeout(5_000) { entered.await() }
        leaving.cancelAndJoin()
        vm.setWifiSsids(listOf("new")); vm.requestWifiFlush()
        assertTrue(vm.state.value.isSaving)
        release.complete(Unit)
        assertTrue(vm.flushWifi())
        assertEquals(2, network.writes)
        assertEquals(listOf("new"), vm.state.value.wifi.ssids)
        assertEquals(listOf("new"), network.wifi.blacklist)
    }

    @Test fun backgroundSaveLeavesNewForegroundEditsUnsubmitted() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        network.before = { entered.complete(Unit); withTimeout(5_000) { release.await() } }
        vm.setWifiSsidMode("blacklist"); vm.requestWifiFlush()
        withTimeout(5_000) { entered.await() }
        vm.setWifiSsids(listOf("new"))
        release.complete(Unit); vm.saved()
        assertEquals(1, network.writes)
        assertTrue(vm.state.value.hasPendingWifi)
        assertTrue(network.wifi.blacklist.isEmpty())
        assertTrue(vm.flushWifi())
        assertEquals(listOf("new"), network.wifi.blacklist)
    }

    @Test fun autoStartSaveKeepsLoadedSettingsAndRejectsDuplicateAndNoOpWrites() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setAutoStartEnabled(true)
        assertEquals(0, network.writes)
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        network.before = { entered.complete(Unit); withTimeout(5_000) { release.await() } }
        vm.setAutoStartEnabled(false)
        try {
            withTimeout(5_000) { entered.await() }
            assertTrue(vm.state.value.hasLoaded)
            assertTrue(vm.state.value.autoStartEnabled)
            assertTrue(vm.state.value.isSaving)
            vm.setAutoStartEnabled(true)
            assertEquals(1, network.writes)
        } finally { release.complete(Unit) }
        vm.saved()
        assertTrue(vm.state.value.hasLoaded)
        assertFalse(vm.state.value.autoStartEnabled)
        assertEquals(1, network.writes)
        assertEquals(2, network.reads)
        vm.setAutoStartEnabled(true); vm.saved()
        assertTrue(vm.state.value.autoStartEnabled)
        assertEquals(12, network.autoStartRevision)
    }

    @Test fun autoStartApplyPreservesDirtyWifiAndWifiRevision() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiSsids(listOf("draft"))
        vm.setAutoStartEnabled(false); vm.saved()
        assertFalse(vm.state.value.autoStartEnabled)
        assertEquals(listOf("draft"), vm.state.value.wifi.ssids)
        assertTrue(vm.state.value.hasPendingWifi)
        assertEquals(0, network.wifiRevision)
        assertEquals(11, network.autoStartRevision)
        assertTrue(vm.flushWifi())
        assertEquals(listOf("draft"), network.wifi.blacklist)
        assertEquals(2, network.reads)
    }

    @Test fun wifiApplyDoesNotConflictWithOrOverwriteConcurrentAutoStartChange() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        network.autoStart = false; network.autoStartRevision++
        vm.setWifiSsidMode("blacklist")
        assertTrue(vm.flushWifi())
        assertFalse(network.autoStart)
        assertEquals(11, network.autoStartRevision)
        vm.refresh(); vm.loaded()
        assertFalse(vm.state.value.autoStartEnabled)
    }

    @Test fun concurrentPartitionSavesKeepBusyStateAndNewDraftInEitherCompletionOrder() = runBlocking {
        for (wifiFirst in listOf(true, false)) {
            val network = Network()
            val vm = network.viewModel(this)
            vm.refresh(); vm.loaded()
            val wifiEntered = CompletableDeferred<Unit>()
            val autoEntered = CompletableDeferred<Unit>()
            val wifiRelease = CompletableDeferred<Unit>()
            val autoRelease = CompletableDeferred<Unit>()
            network.before = { target ->
                if (target == "module/wifi") {
                    wifiEntered.complete(Unit); withTimeout(5_000) { wifiRelease.await() }
                } else {
                    autoEntered.complete(Unit); withTimeout(5_000) { autoRelease.await() }
                }
            }
            vm.setWifiSsidMode("blacklist"); vm.requestWifiFlush()
            vm.setAutoStartEnabled(false)
            try {
                withTimeout(5_000) { wifiEntered.await(); autoEntered.await() }
                vm.setWifiSsids(listOf("new"))
                if (wifiFirst) wifiRelease.complete(Unit) else autoRelease.complete(Unit)
                withTimeout(5_000) { vm.state.first {
                    if (wifiFirst) !it.isSavingWifi else !it.isSavingAutoStart
                } }
                assertTrue(vm.state.value.isSaving)
                assertEquals(!wifiFirst, vm.state.value.isSavingWifi)
                assertEquals(wifiFirst, vm.state.value.isSavingAutoStart)
                assertEquals(listOf("new"), vm.state.value.wifi.ssids)
                assertTrue(vm.state.value.hasPendingWifi)
            } finally { wifiRelease.complete(Unit); autoRelease.complete(Unit) }
            vm.saved()
            assertFalse(vm.state.value.autoStartEnabled)
            assertTrue(vm.state.value.hasPendingWifi)
            assertTrue(network.wifi.blacklist.isEmpty())
            assertEquals(2, network.reads)
            assertEquals(2, network.writes)
            assertTrue(vm.flushWifi())
            assertEquals(listOf("new"), network.wifi.blacklist)
        }
    }

    @Test fun autoStartConflictDoesNotRereadOrDiscardWifiDraft() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiSsids(listOf("draft"))
        network.autoStartRevision++
        vm.setAutoStartEnabled(false); vm.saved()
        assertEquals("conflict", vm.state.value.error)
        assertTrue(vm.state.value.autoStartEnabled)
        assertEquals(listOf("draft"), vm.state.value.wifi.ssids)
        vm.setAutoStartEnabled(false); vm.refresh()
        assertEquals(1, network.writes)
        assertEquals(2, network.reads)
        assertTrue(vm.flushWifi())
        assertEquals(listOf("draft"), network.wifi.blacklist)
        vm.refresh(); vm.loaded()
        vm.setAutoStartEnabled(false); vm.saved()
        assertFalse(vm.state.value.autoStartEnabled)
    }

    @Test fun detachedAutoStartFailureStillNotifiesAndKeepsWifiDraft() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val saveScope = CoroutineScope(coroutineContext + Job())
        val errors = mutableListOf<String>()
        val writes = ConfigurationWrites(saveScope) { errors += it.message.orEmpty() }
        val network = Network()
        val vm = network.viewModel(pageScope, writes)
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        network.before = { entered.complete(Unit); withTimeout(5_000) { release.await() }; error("save failed") }
        try {
            vm.refresh(); vm.loaded()
            vm.setWifiSsids(listOf("draft"))
            vm.setAutoStartEnabled(false)
            withTimeout(5_000) { entered.await() }
            pageScope.cancel()
            assertTrue(vm.state.value.isSavingAutoStart)
            release.complete(Unit)
            withTimeout(5_000) { writes.await("module/wifi") }
            vm.saved()
            assertEquals(listOf("save failed"), errors)
            assertEquals(listOf("draft"), vm.state.value.wifi.ssids)
            assertTrue(vm.state.value.hasPendingWifi)
            assertTrue(vm.state.value.autoStartEnabled)
        } finally { release.complete(Unit); pageScope.cancel(); saveScope.cancel() }
    }

    @Test fun networkLoadingPreservesLoadedValuesAndRecoversFromFirstReadFailure() = runBlocking {
        val network = Network()
        network.wifi = WifiPolicySettings(enabled = true)
        network.beforeRead = { throw NetProxyCtlException("config.read_failed", "read failed") }
        val vm = network.viewModel(this)
        vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.isLoading } }
        assertFalse(vm.state.value.hasLoaded)
        assertEquals("read failed", vm.state.value.error)
        network.beforeRead = {}
        vm.refresh(); vm.loaded()
        assertTrue(vm.state.value.wifi.enabled)
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        network.beforeRead = { entered.complete(Unit); withTimeout(5_000) { release.await() } }
        vm.refresh()
        try {
            withTimeout(5_000) { entered.await() }
            assertTrue(vm.state.value.hasLoaded)
            assertTrue(vm.state.value.wifi.enabled)
            assertTrue(vm.state.value.isLoading)
        } finally { release.complete(Unit) }
        vm.loaded()
        assertTrue(vm.state.value.hasLoaded)
    }
}
