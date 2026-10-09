package com.fanjv.netproxy.feature.settings.data

import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.ConfigurationWrites
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import com.fanjv.netproxy.core.command.ShellConfigFile
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.yield
import com.fanjv.netproxy.feature.settings.presentation.SettingsViewModel
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class ConfigRepositoryTest {
    @get:Rule val temporaryFolder = TemporaryFolder()

    @Test fun reopeningWaitsForDetachedCommitAndReadsItsNewRevision() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val saveScope = CoroutineScope(coroutineContext + Job())
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var content = "WIFI_AUTO_SWITCH=0\n"
        var revision = 0
        var reads = 0
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            val data = if (args[1] == "read") {
                reads++
                kotlinx.serialization.json.JsonObject(mapOf(
                    "content" to kotlinx.serialization.json.JsonPrimitive(content),
                    "revision" to kotlinx.serialization.json.JsonPrimitive("$revision")
                )).toString()
            } else {
                entered.complete(Unit); withTimeout(5_000) { release.await() }
                assertEquals("$revision", args[3])
                content = File(args.last()).readText()
                revision++
                """{"revision":"$revision"}"""
            }
            NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","message":"","data":$data}"""), emptyList())
        })
        val writes = ConfigurationWrites(saveScope)
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root),
            awaitPendingWrites = writes::await)
        val old = SettingsViewModel(repository, pageScope, writes)
        try {
            old.refresh()
            withTimeout(5_000) { old.state.first { it.hasLoaded } }
            old.setWifiAutoSwitch(true); old.requestWifiFlush()
            pageScope.cancel()
            withTimeout(5_000) { entered.await() }
            val reopened = SettingsViewModel(repository, this, writes)
            reopened.refresh(); yield()
            assertTrue(reopened.state.value.isLoading)
            assertEquals(1, reads)
            release.complete(Unit)
            withTimeout(5_000) { reopened.state.first { it.hasLoaded } }
            assertTrue(reopened.state.value.wifi.enabled)
            assertEquals(2, reads)
            assertEquals(1, revision)
            assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
        } finally { release.complete(Unit); pageScope.cancel(); saveScope.cancel() }
    }

    @Test fun booleanValuesAndDefaultsMatchNative() {
        for (value in listOf("1", "true")) assertTrue(ShellConfigFile.boolean(value))
        for (value in listOf("0", "false")) assertFalse(ShellConfigFile.boolean(value, true))
        assertFalse(ShellConfigFile.boolean(null))
        assertTrue(ShellConfigFile.boolean(null, true))
        for (value in listOf("", "yes", "2")) {
            assertThrows(IllegalStateException::class.java) { ShellConfigFile.boolean(value) }
        }
    }

    @Test fun unrelatedConfigurationReadDoesNotWaitForModuleWrite() = runBlocking {
        val release = CompletableDeferred<Unit>()
        val writes = ConfigurationWrites(this)
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("config", "read", "singbox/config.json"), args)
            NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"config.read","message":"","data":{"content":"{}","revision":"main"}}"""), emptyList())
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root),
            awaitPendingWrites = writes::await)
        val writer = writes.launch("module", write = { release.await() })
        try {
            val snapshot = withTimeout(5_000) { repository.readSnapshot("singbox/config.json") }
            assertEquals("main", snapshot.revision)
            assertTrue(writer.isActive)
        } finally { release.complete(Unit); writer.join() }
    }

    @Test fun savesReadRevisionAndNeverRetriesConflict() = runBlocking {
        val calls = mutableListOf<List<String>>()
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            calls.add(args)
            if (args[1] == "read") {
                NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"config.read","message":"配置内容","data":{"target":"module","content":"AUTO_START=0\n","revision":"read-revision"}}"""), emptyList())
            } else {
                assertEquals(listOf("config", "apply", "--revision", "read-revision", "module"), args.dropLast(1))
                assertEquals("AUTO_START=1\n", File(args.last()).readText())
                NetProxyCtlOutput(false, listOf("""{"schema":1,"ok":false,"code":"config.conflict","message":"配置已被修改，请重新加载后再保存"}"""), emptyList())
            }
        })
        val repository = ConfigRepository(client, CommandFileStore(temporaryFolder.root))
        try {
            repository.updateValue("module", "AUTO_START", "1")
            fail("应返回配置冲突")
        } catch (error: NetProxyCtlException) {
            assertEquals("config.conflict", error.resultCode)
            assertTrue(error.message.contains("重新加载"))
        }
        assertEquals(2, calls.size)
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun nativeInboundValidationCarriesRevisionAndCleansCandidate() = runBlocking {
        val content = """{"tun":{"type":"tun","tag":"netproxy-in"}}"""
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            assertEquals(listOf("config", "validate", "--revision", "tun-revision", "inbound/tun"), args.dropLast(1))
            assertEquals(content, File(args.last()).readText())
            NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"config.validated","message":"","data":{}}"""), emptyList())
        })
        ConfigRepository(client, CommandFileStore(temporaryFolder.root)).validate("inbound/tun", content, "tun-revision")
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }
}
