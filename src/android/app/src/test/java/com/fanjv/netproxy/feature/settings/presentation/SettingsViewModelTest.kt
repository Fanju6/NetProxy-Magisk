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
        var module = "AUTO_START=1\nACTIVE_GROUP_ID=default\nWIFI_AUTO_SWITCH=0\n"
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

    @Test fun confirmedWifiEditsMergeOnlyOnLeaveAndNoOpDoesNotWrite() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiAutoSwitch(true)
        vm.setWifiSsidMode("whitelist")
        vm.setWifiSsidList("home， second,home")
        vm.setProxyOnCellular(false)
        assertEquals(0, network.writes)
        assertTrue(vm.state.value.hasPendingWifi)
        assertTrue(vm.flushWifi())
        assertEquals(1, network.writes)
        val saved = ShellConfigFile.parse(network.module)
        assertEquals("home,second", saved["WIFI_SSID_LIST"])
        assertEquals("default", saved["ACTIVE_GROUP_ID"])
        assertEquals("1", saved["AUTO_START"])
        vm.setWifiAutoSwitch(false); vm.setWifiAutoSwitch(true)
        assertFalse(vm.state.value.hasPendingWifi)
        assertTrue(vm.flushWifi())
        assertEquals(1, network.writes)
    }

    @Test fun dirtyResumeAndConflictNeverBorrowNewRevisionOrLoseDraft() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiSsidList("draft")
        network.module = "AUTO_START=0\nWIFI_SSID_LIST=remote\n"
        network.revision++
        vm.refresh()
        assertEquals(1, network.reads)
        assertFalse(vm.flushWifi())
        assertEquals("draft", vm.state.value.wifi.ssids)
        assertEquals("remote", ShellConfigFile.parse(network.module)["WIFI_SSID_LIST"])
        assertFalse(vm.flushWifi())
        assertEquals(1, network.writes)
        vm.discardWifiAndReload(); vm.loaded()
        assertFalse(vm.state.value.hasPendingWifi)
        assertEquals("remote", vm.state.value.wifi.ssids)
        assertFalse(vm.state.value.autoStartEnabled)
    }

    @Test fun queuedSaveOnlyIncludesWifiValuesAtItsTrigger() = runBlocking {
        val network = Network()
        val vm = network.viewModel(this)
        vm.refresh(); vm.loaded()
        vm.setWifiAutoSwitch(true); vm.requestWifiFlush()
        vm.setWifiSsidList("later")
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals("", ShellConfigFile.parse(network.module)["WIFI_SSID_LIST"])
        assertEquals("later", vm.state.value.wifi.ssids)
        assertTrue(vm.state.value.hasPendingWifi)
        assertTrue(vm.flushWifi())
        assertEquals("later", ShellConfigFile.parse(network.module)["WIFI_SSID_LIST"])
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
        vm.setWifiAutoSwitch(true)
        val leaving = launch { vm.flushWifi() }
        withTimeout(5_000) { entered.await() }
        leaving.cancelAndJoin()
        vm.setWifiSsidList("new")
        vm.requestWifiFlush()
        assertTrue(vm.state.value.isSaving)
        release.complete(Unit)
        assertTrue(vm.flushWifi())
        assertEquals(2, network.writes)
        assertEquals("new", vm.state.value.wifi.ssids)
        assertEquals("new", ShellConfigFile.parse(network.module)["WIFI_SSID_LIST"])
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
        vm.setWifiAutoSwitch(true); vm.requestWifiFlush()
        withTimeout(5_000) { entered.await() }
        vm.setWifiSsidList("new")
        release.complete(Unit)
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals(1, network.writes)
        assertTrue(vm.state.value.hasPendingWifi)
        assertNull(ShellConfigFile.parse(network.module)["WIFI_SSID_LIST"]?.takeIf(String::isNotBlank))
        assertTrue(vm.flushWifi())
        assertEquals("new", ShellConfigFile.parse(network.module)["WIFI_SSID_LIST"])
    }

    @Test fun moduleAndWifiSettingsNeverReadOrMapInboundKeys() = runBlocking {
        var module = "AUTO_START=1\nWIFI_AUTO_SWITCH=1\nWIFI_SSID_MODE=whitelist\nWIFI_SSID_LIST=\"example\"\nPROXY_ON_CELLULAR=0\n"
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
        assertEquals("example", vm.state.value.wifi.ssids)
        assertFalse(vm.state.value.wifi.proxyOnCellular)
        vm.setWifiSsidList(" example，second , third ")
        assertEquals(0, calls.count { it[1] == "apply" })
        assertTrue(vm.flushWifi())
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals("example,second,third", vm.state.value.wifi.ssids)
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
            val data = JsonObject(mapOf("content" to JsonPrimitive("WIFI_AUTO_SWITCH=1\n"),
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
