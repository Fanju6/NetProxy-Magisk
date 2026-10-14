package com.fanjv.netproxy.feature.settings.data

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.core.config.ConfigurationWrites
import com.fanjv.netproxy.feature.settings.model.WifiPolicySettings
import com.fanjv.netproxy.feature.settings.presentation.SettingsViewModel
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.yield
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class ConfigRepositoryTest {
    @get:Rule val temporaryFolder = TemporaryFolder()

    private val defaultWifi = """{"wifi":{"enabled":false,"mode":"blacklist","blacklist":[],"whitelist":[],"proxy_on_non_wifi":true}}"""

    private fun output(data: String) = ModuleHttpResponse(200, """{"schema":1,"ok":true,"code":"test","message":"","data":$data}""")

    private fun snapshot(content: String, revision: String) = output(JsonObject(mapOf(
        "content" to JsonPrimitive(content), "revision" to JsonPrimitive(revision)
    )).toString())

    private suspend fun assertModuleReadRejected(target: String, content: String) {
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            args.assertConfig("GET", target)
            snapshot(content, "read")
        })
        val repository = ConfigRepository(client)
        try {
            if (target == "module/wifi") repository.readWifi() else repository.readAutoStart()
            fail("应拒绝无效配置: $content")
        } catch (_: IllegalArgumentException) { }
    }

    @Test fun savedWifiNamesUseReadonlyNetworkContract() = runBlocking {
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            assertEquals("/network/wifi-list", args.path)
            output("""{"ssids":["Home, Wi-Fi","Office"]}""")
        })
        assertEquals(listOf("Home, Wi-Fi", "Office"), ConfigRepository(client).savedWifiNetworks())
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun modulePartitionsDecodeTypedValuesAndIndependentRevisions() = runBlocking {
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            when (args.target) {
                "module/wifi" -> snapshot(defaultWifi, "wifi-read")
                "module/auto_start" -> snapshot("""{"auto_start":false}""", "auto-read")
                else -> error("非分区读取: $args")
            }
        })
        val repository = ConfigRepository(client)
        val wifi = repository.readWifi()
        assertEquals(WifiPolicySettings(), wifi.wifi)
        assertEquals("wifi-read", wifi.revision)
        val auto = repository.readAutoStart()
        assertFalse(auto.enabled)
        assertEquals("auto-read", auto.revision)
    }

    @Test fun modulePartitionWritesContainOnlyOwnedFieldsIncludingDefaults() = runBlocking {
        val writes = ConfigurationWrites(this)
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            assertEquals("PUT", args.method)
            assertEquals("/config", args.path)
            val target = args.target
            assertTrue(target in setOf("module/wifi", "module/auto_start"))
            assertEquals(if (target == "module/wifi") "wifi-read" else "auto-read", args.text("revision"))
            assertEquals(Json.parseToJsonElement(if (target == "module/wifi") defaultWifi else """{"auto_start":false}"""),
                Json.parseToJsonElement(args.text("content")))
            assertEquals(setOf("content", "revision"), args.body.keys)
            output("""{"revision":"$target-saved"}""")
        })
        val repository = ConfigRepository(client, writes::await)
        withTimeout(5_000) {
            writes.launch("module/wifi", write = {
                assertEquals("module/wifi-saved", repository.applyWifi(WifiPolicySettings(), "wifi-read"))
            }).join()
            writes.launch("module/auto_start", write = {
                assertEquals("module/auto_start-saved", repository.applyAutoStart(false, "auto-read"))
            }).join()
        }
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun omittedModuleFieldsUseConstructorDefaultsWithoutWriting() = runBlocking {
        val wifiCases = listOf(
            "{}" to WifiPolicySettings(),
            """{"wifi":{}}""" to WifiPolicySettings(),
            """{"wifi":{"enabled":true}}""" to WifiPolicySettings(enabled = true),
            """{"wifi":{"mode":"whitelist","whitelist":[" Home,Wi-Fi "]}}""" to
                WifiPolicySettings(mode = "whitelist", whitelist = listOf(" Home,Wi-Fi ")),
            """{"wifi":{"proxy_on_non_wifi":false}}""" to WifiPolicySettings(proxyOnNonWifi = false)
        )
        for ((content, expected) in wifiCases) {
            val client = ModuleApiClient(transport = TestApiTransport { args ->
                args.assertConfig("GET", "module/wifi")
                snapshot(content, "sparse-wifi")
            })
            val read = ConfigRepository(client).readWifi()
            assertEquals(expected, read.wifi)
            assertEquals("sparse-wifi", read.revision)
        }
        for ((content, expected) in listOf("{}" to false, """{"auto_start":false}""" to false,
            """{"auto_start":true}""" to true)) {
            val client = ModuleApiClient(transport = TestApiTransport { args ->
                args.assertConfig("GET", "module/auto_start")
                snapshot(content, "sparse-auto")
            })
            val read = ConfigRepository(client).readAutoStart()
            assertEquals(expected, read.enabled)
            assertEquals("sparse-auto", read.revision)
        }
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun nullUnknownAndMalformedModuleJsonAreRejected() = runBlocking {
        val invalidWifi = listOf("""{"wifi":null}""", """{"wifi":{"unknown":true}}""", """{"unknown":true}""",
            """{"wifi":{"enabled":null}}""", """{"wifi":{"mode":null}}""",
            """{"wifi":{"blacklist":null}}""", """{"wifi":{"whitelist":null}}""",
            """{"wifi":{"proxy_on_non_wifi":null}}""",
            defaultWifi.replace("\"enabled\":false", "\"enabled\":0"),
            defaultWifi.replace("\"mode\":\"blacklist\"", "\"mode\":\"off\""),
            defaultWifi.replace("\"blacklist\":[]", "\"blacklist\":[1]"))
        for ((target, invalid) in listOf("module/wifi" to invalidWifi,
            "module/auto_start" to listOf("""{"auto_start":null}""", """{"auto_start":1}""",
                """{"unknown":true}"""))) {
            for (content in invalid) assertModuleReadRejected(target, content)
        }
    }

    @Test fun quotedModuleBooleansAreRejected() = runBlocking {
        for (value in listOf("true", "false")) {
            assertModuleReadRejected("module/auto_start", """{"auto_start":"$value"}""")
            for (field in listOf("enabled", "proxy_on_non_wifi")) {
                assertModuleReadRejected("module/wifi", """{"wifi":{"$field":"$value"}}""")
            }
        }
    }

    @Test fun numericWifiListElementsAreRejected() = runBlocking {
        for (field in listOf("blacklist", "whitelist")) {
            for (values in listOf("[1]", "[\"Home\",2]", "[1.5]")) {
                assertModuleReadRejected("module/wifi", """{"wifi":{"$field":$values}}""")
            }
        }
        val client = ModuleApiClient(transport = TestApiTransport { _ ->
            snapshot("""{"wifi":{"blacklist":["1"],"whitelist":["1.5"]}}""", "read")
        })
        val wifi = ConfigRepository(client).readWifi().wifi
        assertEquals(listOf("1"), wifi.blacklist)
        assertEquals(listOf("1.5"), wifi.whitelist)
    }

    @Test fun strictModuleCodecDoesNotChangeApiResponseUnknownFieldPolicy() = runBlocking {
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            args.assertConfig("GET", "module/auto_start")
            output("""{"content":"{}","revision":"auto-read","extra":true}""")
        })
        assertTrue(client.json.configuration.ignoreUnknownKeys)
        val read = ConfigRepository(client).readAutoStart()
        assertFalse(read.enabled)
        assertEquals("auto-read", read.revision)
        assertTrue(client.json.configuration.ignoreUnknownKeys)
    }

    @Test fun reopeningWaitsForDetachedCommitAndReadsItsNewRevision() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val saveScope = CoroutineScope(coroutineContext + Job())
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var content = defaultWifi
        var revision = 0
        var reads = 0
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            if (args.method == "GET") {
                reads++
                if (args.target == "module/wifi") snapshot(content, "$revision")
                else snapshot("""{"auto_start":false}""", "auto")
            } else {
                entered.complete(Unit); withTimeout(5_000) { release.await() }
                args.assertConfig("PUT", "module/wifi", "$revision")
                content = args.text("content")
                revision++
                output("""{"revision":"$revision"}""")
            }
        })
        val writes = ConfigurationWrites(saveScope)
        val repository = ConfigRepository(client, writes::await)
        val old = SettingsViewModel(repository, pageScope, writes)
        try {
            old.refresh()
            withTimeout(5_000) { old.state.first { it.hasLoaded } }
            old.setWifiSsidMode("blacklist"); old.requestWifiFlush()
            pageScope.cancel()
            withTimeout(5_000) { entered.await() }
            val reopened = SettingsViewModel(repository, this, writes)
            reopened.refresh(); yield()
            assertTrue(reopened.state.value.isLoading)
            assertEquals(2, reads)
            release.complete(Unit)
            withTimeout(5_000) { reopened.state.first { it.hasLoaded } }
            assertTrue(reopened.state.value.wifi.enabled)
            assertEquals(4, reads)
            assertEquals(1, revision)
            assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
        } finally { release.complete(Unit); pageScope.cancel(); saveScope.cancel() }
    }

    @Test fun unrelatedConfigurationReadDoesNotWaitForModuleWrite() = runBlocking {
        val release = CompletableDeferred<Unit>()
        val writes = ConfigurationWrites(this)
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            args.assertConfig("GET", "singbox/config.json")
            snapshot("{}", "main")
        })
        val repository = ConfigRepository(client, writes::await)
        val writer = writes.launch("module/wifi", write = { release.await() })
        try {
            val snapshot = withTimeout(5_000) { repository.readSnapshot("singbox/config.json") }
            assertEquals("main", snapshot.revision)
            assertTrue(writer.isActive)
        } finally { release.complete(Unit); writer.join() }
    }

    @Test fun savesReadPartitionRevisionAndNeverRereadsOrRetriesConflict() = runBlocking {
        val calls = mutableListOf<ApiTestRequest>()
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            calls.add(args)
            if (args.method == "GET") {
                args.assertConfig("GET", "module/auto_start")
                snapshot("""{"auto_start":false}""", "auto-read")
            } else {
                args.assertConfig("PUT", "module/auto_start", "auto-read")
                assertEquals("""{"auto_start":true}""", args.text("content"))
                ModuleHttpResponse(500, """{"schema":1,"ok":false,"code":"config.conflict","message":"配置已被修改，请重新加载后再保存"}""")
            }
        })
        val repository = ConfigRepository(client)
        val read = repository.readAutoStart()
        try {
            repository.applyAutoStart(true, read.revision)
            fail("应返回配置冲突")
        } catch (error: ModuleApiException) {
            assertEquals("config.conflict", error.resultCode)
            assertTrue(error.message.contains("重新加载"))
        }
        assertEquals(2, calls.size)
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }

    @Test fun fullModuleJsonRemainsAvailableWithoutAndroidMerging() = runBlocking {
        val content = JsonObject(Json.parseToJsonElement(defaultWifi).jsonObject + mapOf(
            "auto_start" to JsonPrimitive(false),
            "selection" to JsonObject(mapOf("group_id" to JsonPrimitive("default"), "node_tag" to JsonPrimitive("")))
        )).toString()
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            if (args.method == "GET") {
                args.assertConfig("GET", "module")
                snapshot(content, "full-read")
            } else {
                args.assertConfig("PUT", "module", "full-read")
                assertEquals(content, args.text("content"))
                output("""{"revision":"full-saved"}""")
            }
        })
        val repository = ConfigRepository(client)
        val read = repository.readSnapshot("module")
        assertEquals("full-saved", repository.apply("module", read.content, read.revision))
    }

    @Test fun nativeInboundValidationCarriesRevisionWithoutCandidateFiles() = runBlocking {
        val content = """{"tun":{"type":"tun","tag":"netproxy-in"}}"""
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            args.assertConfig("POST", "inbound/tun", "tun-revision")
            assertEquals(content, args.text("content"))
            output("{}")
        })
        ConfigRepository(client).validate("inbound/tun", content, "tun-revision")
        assertTrue(temporaryFolder.root.listFiles()!!.isEmpty())
    }
}
