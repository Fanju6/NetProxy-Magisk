package com.fanjv.netproxy.feature.traffic

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.api.ModuleHttpResponse
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.feature.traffic.data.CaptureBreakpointConfig
import com.fanjv.netproxy.feature.traffic.data.CaptureBreakpointEdit
import com.fanjv.netproxy.feature.traffic.data.CapturePageCache
import com.fanjv.netproxy.feature.traffic.data.CaptureReplayEdit
import com.fanjv.netproxy.feature.traffic.data.MitmPolicy
import com.fanjv.netproxy.feature.traffic.data.RewriteRule
import com.fanjv.netproxy.feature.traffic.data.TrafficChangedException
import com.fanjv.netproxy.feature.traffic.data.TrafficRepository
import com.fanjv.netproxy.feature.traffic.presentation.replayHeaderLines
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.Rule
import org.junit.rules.TemporaryFolder

class TrafficRepositoryTest {
    @get:Rule val cacheDir = TemporaryFolder()

    @Test fun clearingLastBreakpointSendsEmptyRulesArray() = runBlocking {
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("/traffic/breakpoints", request.path)
            assertEquals("PUT", request.method)
            assertEquals(Json.parseToJsonElement("""{"rules":[]}"""), request.body)
            apiSuccess("""{"timeout_seconds":60,"rules":[]}""")
        }))
        repository.saveBreakpoints(CaptureBreakpointConfig(rules = emptyList()))
    }

    @Test fun readsPausedResponseAndPostsOnlyEditedFields() = runBlocking {
        val id = "A".repeat(26)
        var posts = 0
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals(mapOf("id" to id), request.query)
            when (request.path) {
                "/traffic/paused" -> apiSuccess("""{"id":"$id","phase":"response","method":"GET","url":"https://example.test/","status":200,"headers":{"Content-Type":["text/plain"]},"body":"original"}""")
                "/traffic/resume" -> {
                    assertEquals("POST", request.method)
                    assertEquals(Json.parseToJsonElement("""{"status":202,"body":""}"""), request.body)
                    posts++
                    apiSuccess("{}")
                }
                else -> error("Unexpected request ${request.path}")
            }
        }))
        val pause = repository.pausedDetail(id)
        assertEquals("response", pause.phase)
        assertEquals("original", pause.body)
        repository.resolvePause(id, CaptureBreakpointEdit(status = 202, body = ""), false)
        assertEquals(1, posts)
    }

    @Test fun replayPostsOnceAndDecodesRouteAttribution() = runBlocking {
        val original = "A".repeat(26)
        val id = "B".repeat(26)
        val edit = CaptureReplayEdit("PUT", "https://other.test/path", mapOf("X-Note" to listOf("one", "two")), "edited")
        var posts = 0
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.path == "/traffic/replay") {
                assertEquals("POST", request.method)
                val body = if (posts++ == 0) """{"id":"$original"}""" else
                    """{"id":"$original","edit":{"method":"PUT","url":"https://other.test/path","headers":{"X-Note":["one","two"]},"body":"edited"}}"""
                assertEquals(Json.parseToJsonElement(body), request.body)
                apiSuccess("""{"id":"$id"}""")
            } else {
                assertEquals("/traffic/detail", request.path)
                assertEquals(mapOf("id" to id), request.query)
                apiSuccess("""{"id":"$id","time":"now","method":"PUT","host":"other.test","outbound":"direct","status":200,"duration_ms":1,"replay_of":"$original","replay_route_changed":true,"replay_unattributed":true,"replay_original_exit":{"chain":[{"tag":"Proxy","type":"selector"}]}}""")
            }
        }))
        assertEquals(id, repository.replay(original, null).id)
        val record = repository.replay(original, edit)
        assertEquals(original, record.replayOf)
        assertEquals("Proxy", record.replayOriginalExit?.chain?.single()?.tag)
        assertTrue(record.replayRouteChanged)
        assertTrue(record.replayUnattributed)
        assertEquals(2, posts)
    }

    @Test fun replayHeaderEditorPreservesRepeatedValues() {
        assertEquals(mapOf("X-Note" to listOf("one:two", "three"), "Empty" to listOf("")),
            replayHeaderLines("X-Note: one:two\nX-Note: three\nEmpty:"))
        assertNull(replayHeaderLines("invalid"))
    }

    @Test fun captureRecordsDecodeCertificateDiagnostics() = runBlocking {
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport {
            apiSuccess("""{"running":true,"count":1,"entries":[{"time":"now","method":"TLS","host":"example.test","outbound":"direct","status":0,"duration_ms":1,"tls":"pinned_skipped","passthrough_reason":"client_ca_untrusted","ca_stores":{"user":"present","system":"absent"}}]}""")
        }))
        val record = repository.preview().entries.single()
        assertEquals("client_ca_untrusted", record.passthroughReason)
        assertEquals("present", record.caStores?.user)
        assertEquals("absent", record.caStores?.system)
    }

    @Test fun capturedDomainsAndFilteredPagesUseBatchQueriesAndDoNotPolluteCache() = runBlocking {
        val batch = "A".repeat(26)
        val cache = CapturePageCache(cacheDir.root, Json)
        var pages = 0
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.path == "/traffic/domains") {
                assertEquals(mapOf("batch" to batch), request.query)
                apiSuccess("""[{"host":"example.test","count":2}]""")
            } else {
                pages++
                assertEquals(mapOf("batch" to batch, "before" to "3") +
                    if (pages == 1) mapOf("host" to "example.test") else emptyMap(), request.query)
                apiSuccess("""{"entries":[],"batch":"$batch","oldest":"1"}""")
            }
        }), cache)
        assertEquals(2, repository.domains(batch).single().count)
        repository.page(batch, before = "3", oldest = "1", host = "example.test")
        repository.page(batch, before = "3", oldest = "1")
        assertEquals(2, pages)
    }

    @Test fun rewriteRulesDefaultToEnabledAndPersistExplicitDisable() = runBlocking {
        val rule = """{"pattern":"/inspect","phase":"request","script":"return {}","domains":["example.test"]}"""
        var stored = """{"rules":[$rule]}"""
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("/traffic/rules", request.path)
            if (request.method == "PUT") stored = request.body.toString()
            apiSuccess(stored)
        }))
        val original = repository.rules()
        assertTrue(original.single().enabled)
        val disabled = original.map { it.copy(enabled = false) }
        assertEquals(disabled, repository.saveRules(original, disabled))
        assertTrue(stored.contains("\"enabled\":false"))
        assertEquals(original, repository.saveRules(disabled, original))
    }

    @Test fun rewriteTitlesAreOptionalAndSurviveUpdate() = runBlocking {
        var stored = """{"rules":[{"pattern":"/get","phase":"request","script":"return {}","domains":["example.test"]}]}"""
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.method == "PUT") stored = request.body.toString()
            apiSuccess(stored)
        }))
        val original = repository.rules()
        assertEquals("", original.single().name)
        val titled = original.map { it.copy(name = "Add test header") }
        assertEquals(titled, repository.saveRules(original, titled))
        assertTrue(stored.contains("\"name\":\"Add test header\""))
        assertEquals(original, repository.saveRules(titled, original))
        assertTrue(!stored.contains("\"name\""))
    }

    @Test fun overlayPreviewUsesOneReadAndDecodesSessionTotal() = runBlocking {
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("GET", request.method)
            assertEquals("/traffic/preview", request.path)
            apiSuccess("""{"running":true,"count":201,"entries":[{"time":"now","method":"GET","host":"example.test","outbound":"direct","status":200,"duration_ms":1}]}""")
        }))
        val preview = repository.preview()
        assertTrue(preview.running)
        assertEquals(201, preview.count)
        assertEquals("example.test", preview.entries.single().host)
    }

    @Test fun captureUsesModulePathsAndDecodesRecordMetadata() = runBlocking {
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            when (request.method to request.path) {
                "POST" to "/traffic/start" -> {
                    assertTrue(request.mutation)
                    apiSuccess("""{"running":true,"count":0}""")
                }
                "GET" to "/traffic/list" -> apiSuccess(
                    """[{"time":"2026-01-01T00:00:00Z","method":"GET","host":"example.test","status":200,"outbound":"direct","duration_ms":15,"tls":"decrypted"}]"""
                )
                "DELETE" to "/traffic/list" -> {
                    assertTrue(request.mutation)
                    apiSuccess("""{"running":true,"count":0}""")
                }
                else -> error("Unexpected request: ${request.method} ${request.path}")
            }
        }))
        assertTrue(repository.setCapture(true).running)
        val record = repository.records().single()
        assertEquals("decrypted", record.tls)
        assertEquals("direct", record.outbound)
        assertEquals(15L, record.durationMs)
        assertEquals(0, repository.clear().count)
    }

    @Test fun detailReadsHeadersAndEncodedBodiesOnDemand() = runBlocking {
        val id = "AAAAAAAAAAAAAAAAAAAAAAAAAA"
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("GET", request.method)
            assertEquals("/traffic/detail", request.path)
            assertEquals(mapOf("id" to id), request.query)
            apiSuccess("""
                {"id":"$id","http":{"url":"https://example.test/form?q=1","url_truncated":false,
                "request":{"headers":{"Content-Type":["text/plain"]},"headers_truncated":false,
                "body":{"data":"hello","format":"text","truncated":false}},
                "response":{"headers":{"Set-Cookie":["a=1","b=2"]},"headers_truncated":true,
                "body":{"data":"Cg==","format":"base64","content_type":"application/octet-stream","content_encoding":"br","truncated":true}}}}
            """.trimIndent())
        }))
        val detail = requireNotNull(repository.detail(id).http)
        assertEquals("https://example.test/form?q=1", detail.url)
        assertEquals("hello", detail.request.body.data)
        assertEquals(listOf("a=1", "b=2"), detail.response.headers["Set-Cookie"])
        assertTrue(detail.response.headersTruncated)
        assertTrue(detail.response.body.truncated)
        assertEquals("base64", detail.response.body.format)
        assertEquals("br", detail.response.body.contentEncoding)
    }

    @Test fun detailDecodesProtocolTrailersAndNestedMessages() = runBlocking {
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("/traffic/detail", request.path)
            apiSuccess("""
                {"id":"AAAAAAAAAAAAAAAAAAAAAAAAAA","http":{"url":"https://example.test/example.Echo/Send","url_truncated":false,
                "request":{"headers":{},"headers_truncated":false,"body":{"data":"","format":"base64","truncated":false}},
                "response":{"headers":{},"headers_truncated":false,"trailers":{"Grpc-Status":["3"]},
                "grpc":{"status":"3","message":"sample error","messages":[{"size":4,"fields":[{"number":3,"wire":2,
                "fields":[{"number":1,"wire":0,"value":"7"}]}]}]},
                "body":{"data":"","format":"base64","truncated":false}},
                "websocket":{"frames":[{"direction":"receive","opcode":8,"size":2,"close_code":1000,"truncated":false}],
                "messages":[{"direction":"receive","opcode":1,"size":8,"text":"example","compressed":true,"truncated":false}],
                "truncated":false}}}
            """.trimIndent())
        }))
        val detail = requireNotNull(repository.detail("AAAAAAAAAAAAAAAAAAAAAAAAAA").http)
        assertEquals(listOf("3"), detail.response.trailers["Grpc-Status"])
        val grpc = requireNotNull(detail.response.grpc)
        assertEquals("sample error", grpc.message)
        assertEquals("7", grpc.messages.single().fields.single().fields.single().value)
        val websocket = requireNotNull(detail.websocket)
        assertEquals(1000, websocket.frames.single().closeCode)
        assertTrue(websocket.messages.single().compressed)
    }

    @Test fun batchOperationsUseExplicitIDs() = runBlocking {
        val id = "AAAAAAAAAAAAAAAAAAAAAAAAAA"
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            when (request.method to request.path) {
                "GET" to "/traffic/batches" -> apiSuccess(
                    """[{"id":"$id","started_at":"2026-01-01T00:00:00Z","count":2,"running":false}]"""
                )
                "GET" to "/traffic/list" -> {
                    assertEquals(mapOf("batch" to id), request.query)
                    apiSuccess("[]")
                }
                "DELETE" to "/traffic/batches" -> {
                    assertEquals(mapOf("id" to id), request.query)
                    assertTrue(request.mutation)
                    apiSuccess("null")
                }
                else -> error("Unexpected request: ${request.method} ${request.path}")
            }
        }))
        assertEquals(id, repository.batches().single().id)
        assertTrue(repository.records(id).isEmpty())
        repository.deleteBatch(id)
    }

    @Test fun capturePagesReuseCacheOnlyForSameRetentionWindowAndInvalidateOnClear() = runBlocking {
        val id = "AAAAAAAAAAAAAAAAAAAAAAAAAA"
        val cache = CapturePageCache(cacheDir.root, Json)
        var reads = 0
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            when (request.method to request.path) {
                "GET" to "/traffic/page" -> {
                    assertEquals(mapOf("batch" to id, "before" to "20"), request.query)
                    reads++
                    apiSuccess("""{"batch":"$id","entries":[],"before":"10","oldest":"1"}""")
                }
                "DELETE" to "/traffic/list" -> apiSuccess("""{"running":false,"count":0}""")
                else -> error("Unexpected request: ${request.method} ${request.path}")
            }
        }), cache)
        assertEquals("10", repository.page(id, before = "20", oldest = "1").before)
        assertEquals("10", repository.page(id, before = "20", oldest = "1").before)
        assertEquals(1, reads)
        assertEquals("1", repository.page(id, before = "20", oldest = "2").oldest)
        assertEquals(2, reads)
        repository.clear()
        assertEquals(null, cache.read(id, "1", "20"))
    }

    @Test fun replaceRulesDoesNotOverwriteAChangedListAndReportsValidation() = runBlocking {
        val existing = RewriteRule("example", "request", "return {};", listOf("example.org"))
        val invalid = existing.copy(pattern = "[")
        var writes = 0
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("/traffic/rules", request.path)
            if (request.method == "GET") {
                apiSuccess("""{"rules":[{"pattern":"example","phase":"request","script":"return {};","domains":["example.org"]}]}""")
            } else {
                assertEquals("PUT", request.method)
                assertTrue(request.mutation)
                assertEquals("""{"rules":[{"pattern":"[","phase":"request","script":"return {};","domains":["example.org"]}]}""", request.body.toString())
                writes++
                ModuleHttpResponse(422, """{"schema":1,"ok":false,"code":"traffic.rules_failed","message":"Invalid URL expression"}""")
            }
        }))
        assertTrue(runCatching { repository.saveRules(emptyList(), listOf(invalid)) }.exceptionOrNull() is TrafficChangedException)
        assertEquals(0, writes)
        val error = runCatching { repository.saveRules(listOf(existing), listOf(invalid)) }.exceptionOrNull() as ModuleApiException
        assertEquals("traffic.rules_failed", error.resultCode)
        assertEquals("Invalid URL expression", error.message)
        assertEquals(1, writes)
    }

    @Test fun systemTrustRemovalErrorIncludesPersistedPolicy() = runBlocking {
        val expected = MitmPolicy(true, listOf("example.test"), true)
        val saved = expected.copy(systemTrust = false)
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("/traffic/mitm", request.path)
            if (request.method == "GET") {
                apiSuccess("""{"enabled":true,"hosts":["example.test"],"system_trust":true}""")
            } else {
                assertEquals("PUT", request.method)
                assertTrue(request.mutation)
                assertEquals("""{"enabled":true,"hosts":["example.test"],"system_trust":false}""", request.body.toString())
                ModuleHttpResponse(409, """{"schema":1,"ok":false,"code":"traffic.trust_remove_incomplete","message":"Reboot","data":{"enabled":true,"hosts":["example.test"],"system_trust":false}}""")
            }
        }))
        val error = runCatching { repository.savePolicy(expected, saved) }.exceptionOrNull() as ModuleApiException
        assertEquals(saved, repository.persistedPolicy(error))
    }

    @Test fun certificateExportRefusesCombinedPrivateKeyMaterial() = runBlocking {
        var privateKey = false
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("POST", request.method)
            assertEquals("/traffic/ca-init", request.path)
            val extra = if (privateKey) """\n-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----""" else ""
            apiSuccess("""{"filename":"abcdef01.0","sha256":"${"a".repeat(64)}","pem":"-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n$extra"}""")
        }))
        assertEquals("abcdef01.0", repository.certificate().filename)
        privateKey = true
        val error = runCatching { repository.certificate() }.exceptionOrNull() as ModuleApiException
        assertEquals("traffic.ca_invalid", error.resultCode)
    }
}
