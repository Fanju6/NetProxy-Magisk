package com.fanjv.netproxy.feature.traffic

import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.CaptureSourceApp
import com.fanjv.netproxy.feature.traffic.data.CaptureExit
import com.fanjv.netproxy.feature.traffic.data.CaptureHop
import com.fanjv.netproxy.feature.traffic.data.CapturedBody
import com.fanjv.netproxy.feature.traffic.data.CapturedHttp
import com.fanjv.netproxy.feature.traffic.data.CapturedMessage
import com.fanjv.netproxy.feature.traffic.presentation.SessionGrouping
import com.fanjv.netproxy.feature.traffic.presentation.asCurl
import com.fanjv.netproxy.feature.traffic.presentation.bodyChunks
import com.fanjv.netproxy.feature.traffic.presentation.displayBody
import com.fanjv.netproxy.feature.traffic.presentation.groupSessions
import com.fanjv.netproxy.feature.traffic.presentation.matchesSession
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TrafficPresentationTest {
    @Test fun groupsByAndroidProfileHostAndActualOutboundChain() {
        val first = CaptureRecord("now", "GET", "EXAMPLE.test.:443", "Proxy", 200, durationMs = 1,
            sourceApp = CaptureSourceApp(10123, 0, "com.example.app"),
            exit = CaptureExit(chain = listOf(CaptureHop("Proxy", "selector"), CaptureHop("node-x", "vless"))))
        val second = first.copy(host = "example.test", sourceApp = CaptureSourceApp(1110123, 11, "com.example.app"),
            exit = CaptureExit(chain = listOf(CaptureHop("node-y", "vless"))))
        val third = first.copy(host = "[2001:db8::1]:443", outbound = "direct", sourceApp = null, exit = null)
        val fourth = third.copy(outbound = "block")
        val records = listOf(first, second, third, fourth)
        assertEquals(mapOf(null to records), records.groupSessions(SessionGrouping.None))
        assertEquals(mapOf("0:com.example.app" to listOf(first), "11:com.example.app" to listOf(second),
            null to listOf(third, fourth)), records.groupSessions(SessionGrouping.App))
        assertEquals(mapOf("example.test" to listOf(first, second), "2001:db8::1" to listOf(third, fourth)),
            records.groupSessions(SessionGrouping.Domain))
        assertEquals(listOf("Proxy → node-x", "Proxy → node-y", "direct", "block"),
            records.groupSessions(SessionGrouping.Outbound).keys.toList())
    }

    @Test fun filtersByActualRewriteAppAndSelectedNode() {
        val record = CaptureRecord("now", "GET", "example.test", "Proxy", 200, durationMs = 1,
            rewritten = true, exit = CaptureExit(chain = listOf(CaptureHop("Auto", "urltest"),
                CaptureHop("node-x", "direct")), packageName = "com.example.app"))
        assertTrue(record.matchesSession("", null, null, null, true, "com.example.app", "node-x"))
        assertFalse(record.copy(rewritten = false).matchesSession("", null, null, null, rewrittenOnly = true))
        assertFalse(record.matchesSession("", null, null, null, outbound = "node-y"))
        assertFalse(record.matchesSession("", null, null, null, app = "com.example.other"))
    }

    @Test fun bodyChunksPreserveLineBreaksAndUnicode() {
        val data = "prefix\n" + "x".repeat(2047) + "🚀\nend"
        val chunks = bodyChunks(data)
        assertEquals(data, chunks.joinToString(""))
        assertTrue(chunks.all { it.length <= 2048 && !it.last().isHighSurrogate() })
    }

    @Test fun appFilterDistinguishesAndroidUsers() {
        val record = CaptureRecord("now", "GET", "example.test", "Proxy", 200, durationMs = 1,
            sourceApp = CaptureSourceApp(1010123, 10, "com.example.app"))
        assertTrue(record.matchesSession("", null, null, null, app = "10:com.example.app"))
        assertFalse(record.matchesSession("", null, null, null, app = "0:com.example.app"))
    }

    @Test fun sessionSearchMatchesPathAndCombinesFilters() {
        val record = CaptureRecord("2026-01-01T00:00:00Z", "POST", "example.test", "direct", 201,
            tls = "decrypted", durationMs = 2, target = "/orders?q=new")
        assertTrue(record.matchesSession("HTTPS://example.test/orders", "POST", 2, "decrypted"))
        assertFalse(record.matchesSession("/orders", "GET", 2, "decrypted"))
        assertFalse(record.matchesSession("/orders", "POST", 2, "tunnel"))
        assertFalse(record.matchesSession("/orders", "POST", 4, "decrypted"))
    }

    @Test fun bodyFormattingUsesJsonContentTypeAndKeepsInvalidData() {
        assertEquals("{\n    \"ok\": true\n}", displayBody("{\"ok\":true}", "application/problem+json; charset=utf-8"))
        assertEquals("{invalid", displayBody("{invalid", "application/json"))
        assertEquals("{\"ok\":true}", displayBody("{\"ok\":true}", "text/plain"))
    }

    @Test fun curlQuotesArgumentsAndStreamsEncodedBodyWithoutContentLength() {
        val body = CapturedBody("AQI=", "base64", "application/octet-stream", truncated = true)
        val message = CapturedMessage(mapOf("X-Test" to listOf("don't"), "Content-Length" to listOf("42")), false, body)
        val detail = CapturedHttp("https://example.test/a?b=c&d=e", false, message, message)
        val command = asCurl("POST", detail)
        assertTrue(command.startsWith("# Capture incomplete"))
        assertTrue(command.contains("printf %s 'AQI=' | base64 -d | curl"))
        assertTrue(command.contains("--header 'X-Test: don'\"'\"'t'"))
        assertTrue(command.contains("--data-binary @- -- 'https://example.test/a?b=c&d=e'"))
        assertFalse(command.contains("Content-Length"))
    }
}
