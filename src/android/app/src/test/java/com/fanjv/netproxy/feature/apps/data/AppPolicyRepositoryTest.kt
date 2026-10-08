package com.fanjv.netproxy.feature.apps.data

import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import com.fanjv.netproxy.core.command.CommandFileStore
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import java.io.File
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonArray
import org.junit.Assert.*
import org.junit.Test
import org.junit.Rule
import org.junit.rules.TemporaryFolder

class AppPolicyRepositoryTest {
    @get:Rule val folder = TemporaryFolder()

    @Test fun appPartitionUsesArraysAndReadRevisionWithoutSeparateServiceCommand() = runBlocking {
        val calls = mutableListOf<List<String>>()
        val repository = AppPolicyRepository(ConfigRepository(NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            calls += args
            if (args[1] == "apply") {
                assertEquals(listOf("config", "apply", "--revision", "read-version", "inbound/app"), args.dropLast(1))
                val app = Json.parseToJsonElement(File(args.last()).readText()).jsonObject.getValue("app").jsonObject
                assertEquals(2, app.getValue("proxy_apps").jsonArray.size)
                assertFalse(app.containsKey("revision"))
                return@NetProxyCtlTransport NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"config.apply","message":"","data":{"revision":"saved-version"}}"""), emptyList())
            }
            val content = """{"app":{"enabled":true,"mode":"blacklist","proxy_apps":["0:com.example.client","10:com.example.client"],"bypass_apps":["10:com.example.bypass"]}}"""
            NetProxyCtlOutput(true, listOf("""
                {"schema":1,"ok":true,"code":"config.read","message":"","data":{"content":${Json.encodeToString(kotlinx.serialization.serializer<String>(), content)},"revision":"read-version"}}
            """.trimIndent()), emptyList())
        }), CommandFileStore(folder.root)))
        val config = repository.config()
        assertEquals(listOf("0:com.example.client", "10:com.example.client"), config.proxyApps)
        assertEquals(listOf("10:com.example.bypass"), config.bypassApps)
        assertEquals("saved-version", repository.apply(config.copy(mode = "whitelist")).revision)
        assertEquals(listOf("read", "apply"), calls.map { it[1] })
    }
}
