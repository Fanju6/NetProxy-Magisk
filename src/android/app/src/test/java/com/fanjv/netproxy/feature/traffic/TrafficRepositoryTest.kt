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

    @Test fun replaceRulesDoesNotOverwriteAChangedListAndReportsValidation() = runBlocking {
        val existing = RewriteRule("example", "request", "return {};")
        val invalid = existing.copy(pattern = "[")
        var writes = 0
        val repository = TrafficRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("/traffic/rules", request.path)
            if (request.method == "GET") {
                apiSuccess("""{"rules":[{"pattern":"example","phase":"request","script":"return {};"}]}""")
            } else {
                assertEquals("PUT", request.method)
                assertTrue(request.mutation)
                assertEquals("""{"rules":[{"pattern":"[","phase":"request","script":"return {};"}]}""", request.body.toString())
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
