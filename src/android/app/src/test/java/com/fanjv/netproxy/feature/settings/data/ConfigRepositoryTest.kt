package com.fanjv.netproxy.feature.settings.data

import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import com.fanjv.netproxy.core.command.ShellConfigFile
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class ConfigRepositoryTest {
    @get:Rule val temporaryFolder = TemporaryFolder()

    @Test fun booleanValuesAndDefaultsMatchNative() {
        for (value in listOf("1", "true")) assertTrue(ShellConfigFile.boolean(value))
        for (value in listOf("0", "false")) assertFalse(ShellConfigFile.boolean(value, true))
        assertFalse(ShellConfigFile.boolean(null))
        assertTrue(ShellConfigFile.boolean(null, true))
        for (value in listOf("", "yes", "2")) {
            assertThrows(IllegalStateException::class.java) { ShellConfigFile.boolean(value) }
        }
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
