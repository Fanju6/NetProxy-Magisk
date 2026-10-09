package com.fanjv.netproxy.feature.settings.presentation

import com.fanjv.netproxy.core.command.*
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.launch
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

class SettingsViewModelTest {
    @get:Rule val folder = TemporaryFolder()

    private inner class Network {
        var module = "AUTO_START=1\nACTIVE_GROUP_ID=default\nWIFI_AUTO_SWITCH=0\nWIFI_SSID_MODE=blacklist\n"
        var revision = 0
        var writes = 0
        var reads = 0
        var before: suspend () -> Unit = {}
        fun viewModel(scope: kotlinx.coroutines.CoroutineScope) = SettingsViewModel(ConfigRepository(
            NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
                val data = if (args[1] == "read") {
                    reads++
                    JsonObject(mapOf("content" to JsonPrimitive(module), "revision" to JsonPrimitive("$revision"))).toString()
                } else {
                    writes++
                    before()
                    if (args[3] != "$revision") throw NetProxyCtlException("config.conflict", "conflict")
                    module = File(args.last()).readText()
                    revision++
                    """{"revision":"$revision"}"""
                }
                NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
            }), CommandFileStore(folder.root)), scope)
    }

    private suspend fun SettingsViewModel.loaded() = withTimeout(5_000) { state.first { it.hasLoaded && !it.isLoading } }

    private suspend fun SettingsViewModel.flushWifi(): Boolean = withTimeout(5_000) {
        do {
            requestWifiFlush()
            state.first { !it.isSaving }
        } while (state.value.hasPendingWifi && !state.value.requiresReload)
        !state.value.requiresReload && !state.value.hasPendingWifi
    }

    @Test fun wifiSelectionDerivesFromSwitchWithoutChangingSavedMode() = runBlocking {
        for (value in listOf(null, "0", "false", "1", "true")) {
            val network = Network()
            network.module = "WIFI_SSID_MODE=whitelist\n" +
                if (value == null) "" else "WIFI_AUTO_SWITCH=$value\n"
            val vm = network.viewModel(this)
            vm.refresh(); vm.loaded()
            val enabled = value == "1" || value == "true"
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
        val saved = ShellConfigFile.parse(network.module)
        assertEquals("[\"home\",\"second\"]", saved["WIFI_SSID_WHITELIST"])
        assertEquals("1", saved["WIFI_AUTO_SWITCH"])
        assertEquals("whitelist", saved["WIFI_SSID_MODE"])
        assertEquals("default", saved["ACTIVE_GROUP_ID"])
        assertEquals("1", saved["AUTO_START"])
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
        network.module = "AUTO_START=0\nWIFI_SSID_BLACKLIST=[\"remote\"]\n"
        network.revision++
        vm.refresh()
        assertEquals(1, network.reads)
        assertFalse(vm.flushWifi())
        assertEquals(listOf("draft"), vm.state.value.wifi.ssids)
        assertEquals("[\"remote\"]", ShellConfigFile.parse(network.module)["WIFI_SSID_BLACKLIST"])
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
        val blacklist = listOf(" Home,Wi-Fi ", "办公\"网络")
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
        assertEquals("whitelist", vm.state.value.wifi.mode)
        val saved = ShellConfigFile.parse(network.module)
        assertEquals("0", saved["WIFI_AUTO_SWITCH"])
        assertEquals("whitelist", saved["WIFI_SSID_MODE"])
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
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals("[]", ShellConfigFile.parse(network.module)["WIFI_SSID_BLACKLIST"])
        assertEquals(listOf("later"), vm.state.value.wifi.ssids)
        assertTrue(vm.state.value.hasPendingWifi)
        assertTrue(vm.flushWifi())
        assertEquals("[\"later\"]", ShellConfigFile.parse(network.module)["WIFI_SSID_BLACKLIST"])
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
        vm.setWifiSsids(listOf("new"))
        vm.requestWifiFlush()
        assertTrue(vm.state.value.isSaving)
        release.complete(Unit)
        assertTrue(vm.flushWifi())
        assertEquals(2, network.writes)
        assertEquals(listOf("new"), vm.state.value.wifi.ssids)
        assertEquals("[\"new\"]", ShellConfigFile.parse(network.module)["WIFI_SSID_BLACKLIST"])
    }

    @Test fun backgroundSaveLeavesNewForegroundEditsUnsubmitted() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        network.before = { if (network.writes == 1) {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        vm.setWifiSsidMode("blacklist"); vm.requestWifiFlush()
        withTimeout(5_000) { entered.await() }
        vm.setWifiSsids(listOf("new"))
        release.complete(Unit)
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals(1, network.writes)
        assertTrue(vm.state.value.hasPendingWifi)
        assertEquals("[]", ShellConfigFile.parse(network.module)["WIFI_SSID_BLACKLIST"])
        assertTrue(vm.flushWifi())
        assertEquals("[\"new\"]", ShellConfigFile.parse(network.module)["WIFI_SSID_BLACKLIST"])
    }

    @Test fun moduleAndWifiSettingsNeverReadOrMapInboundKeys() = runBlocking {
        var module = "AUTO_START=1\nWIFI_AUTO_SWITCH=1\nWIFI_SSID_MODE=whitelist\nWIFI_SSID_WHITELIST=[\"example\"]\nPROXY_ON_NON_WIFI=0\n"
        val calls = mutableListOf<List<String>>()
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            calls += args
            val data = if (args[1] == "read") {
                assertEquals(listOf("config", "read", "module"), args)
                JsonObject(mapOf("content" to JsonPrimitive(module), "revision" to JsonPrimitive("module-read"))).toString()
            } else {
                assertEquals(listOf("config", "apply", "--revision", "module-read", "module"), args.dropLast(1))
                module = File(args.last()).readText()
                """{"revision":"module-saved"}"""
            }
            NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
        })
        val vm = SettingsViewModel(ConfigRepository(client, CommandFileStore(folder.root)), this)
        assertFalse(vm.state.value.hasLoaded)
        vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.isLoading } }
        assertTrue(vm.state.value.autoStartEnabled)
        assertTrue(vm.state.value.hasLoaded)
        assertTrue(vm.state.value.wifi.enabled)
        assertEquals("whitelist", vm.state.value.wifi.mode)
        assertEquals(listOf("example"), vm.state.value.wifi.ssids)
        assertFalse(vm.state.value.wifi.proxyOnNonWifi)
        vm.setWifiSsids(listOf("example", "second", "third"))
        assertEquals(0, calls.count { it[1] == "apply" })
        assertTrue(vm.flushWifi())
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals(listOf("example", "second", "third"), vm.state.value.wifi.ssids)
        assertFalse(calls.any { it.contains("ebpf") || it.any { arg -> arg.startsWith("inbound") } })
    }

    @Test fun autoStartSaveKeepsLoadedSettingsAndRejectsDuplicateWrites() = runBlocking {
        var module = "AUTO_START=1\n"
        var writes = 0
        val entered = CompletableDeferred<Unit>()
        val release = CountDownLatch(1)
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            val data = if (args[1] == "read") {
                JsonObject(mapOf("content" to JsonPrimitive(module),
                    "revision" to JsonPrimitive("module-read"))).toString()
            } else {
                writes++
                entered.complete(Unit)
                check(release.await(5, TimeUnit.SECONDS))
                module = File(args.last()).readText()
                """{"revision":"module-saved"}"""
            }
            NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
        })
        val vm = SettingsViewModel(ConfigRepository(client, CommandFileStore(folder.root)), this)
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.hasLoaded && !it.isLoading } }
        vm.setAutoStartEnabled(false)
        try {
            withTimeout(5_000) { entered.await() }
            assertTrue(vm.state.value.hasLoaded)
            assertTrue(vm.state.value.autoStartEnabled)
            assertTrue(vm.state.value.isSaving)
            vm.setAutoStartEnabled(true)
            assertEquals(1, writes)
        } finally {
            release.countDown()
        }
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertTrue(vm.state.value.hasLoaded)
        assertFalse(vm.state.value.autoStartEnabled)
        assertEquals(1, writes)
    }

    @Test fun networkLoadingPreservesLoadedValuesAndRecoversFromFirstReadFailure() = runBlocking {
        var fail = true
        var gate: CountDownLatch? = null
        val entered = CompletableDeferred<Unit>()
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { _, _ ->
            gate?.let {
                entered.complete(Unit)
                check(it.await(5, TimeUnit.SECONDS))
            }
            if (fail) throw NetProxyCtlException("config.read_failed", "read failed")
            val data = JsonObject(mapOf("content" to JsonPrimitive("WIFI_AUTO_SWITCH=1\nWIFI_SSID_MODE=blacklist\n"),
                "revision" to JsonPrimitive("module-read")))
            NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
        })
        val vm = SettingsViewModel(ConfigRepository(client, CommandFileStore(folder.root)), this)
        vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.isLoading } }
        assertFalse(vm.state.value.hasLoaded)
        assertEquals("read failed", vm.state.value.error)
        fail = false
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.hasLoaded && !it.isLoading } }
        assertTrue(vm.state.value.wifi.enabled)
        gate = CountDownLatch(1)
        vm.refresh()
        withTimeout(5_000) { entered.await() }
        assertTrue(vm.state.value.hasLoaded)
        assertTrue(vm.state.value.wifi.enabled)
        assertTrue(vm.state.value.isLoading)
        gate.countDown()
        withTimeout(5_000) { vm.state.first { !it.isLoading } }
        assertTrue(vm.state.value.hasLoaded)
    }
}
