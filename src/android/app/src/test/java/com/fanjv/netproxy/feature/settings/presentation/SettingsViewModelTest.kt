package com.fanjv.netproxy.feature.settings.presentation

import com.fanjv.netproxy.core.command.*
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.CompletableDeferred
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
        withTimeout(5_000) { vm.state.first { !it.isSaving } }
        assertEquals("example,second,third", vm.state.value.wifi.ssids)
        assertFalse(calls.any { it.contains("ebpf") || it.any { arg -> arg.startsWith("inbound") } })
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
