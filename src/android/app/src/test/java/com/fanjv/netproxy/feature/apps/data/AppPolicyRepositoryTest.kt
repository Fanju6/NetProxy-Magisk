package com.fanjv.netproxy.feature.apps.data

import com.fanjv.netproxy.core.api.*

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
        val calls = mutableListOf<ApiTestRequest>()
        val repository = AppPolicyRepository(ConfigRepository(ModuleApiClient(transport = TestApiTransport { args ->
            calls += args
            if (args.method == "PUT") {
                args.assertConfig("PUT", "inbound/app", "read-version")
                val app = Json.parseToJsonElement(args.text("content")).jsonObject.getValue("app").jsonObject
                assertEquals(2, app.getValue("proxy_apps").jsonArray.size)
                assertFalse(app.containsKey("revision"))
                return@TestApiTransport ModuleHttpResponse(200, """{"schema":1,"ok":true,"code":"config.apply","message":"","data":{"revision":"saved-version"}}""")
            }
            val content = """{"app":{"enabled":true,"mode":"blacklist","proxy_apps":["0:com.example.client","10:com.example.client"],"bypass_apps":["10:com.example.bypass"]}}"""
            ModuleHttpResponse(200, """
                {"schema":1,"ok":true,"code":"config.read","message":"","data":{"content":${Json.encodeToString(kotlinx.serialization.serializer<String>(), content)},"revision":"read-version"}}
            """.trimIndent())
        })))
        val config = repository.config()
        assertEquals(listOf("0:com.example.client", "10:com.example.client"), config.proxyApps)
        assertEquals(listOf("10:com.example.bypass"), config.bypassApps)
        assertEquals("saved-version", repository.apply(config.copy(mode = "whitelist")).revision)
        assertEquals(listOf("GET", "PUT"), calls.map { it.method })
    }
}
