package com.fanjv.netproxy.feature.traffic

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.api.ModuleHttpResponse
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.feature.traffic.data.MitmPolicy
import com.fanjv.netproxy.feature.traffic.data.RewriteRule
import com.fanjv.netproxy.feature.traffic.data.TrafficChangedException
import com.fanjv.netproxy.feature.traffic.data.TrafficRepository
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TrafficRepositoryTest {
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
