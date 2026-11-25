package com.fanjv.netproxy.feature.traffic

import com.fanjv.netproxy.feature.traffic.data.CaptureRecord
import com.fanjv.netproxy.feature.traffic.data.CapturedBody
import com.fanjv.netproxy.feature.traffic.data.CapturedHttp
import com.fanjv.netproxy.feature.traffic.data.CapturedMessage
import com.fanjv.netproxy.feature.traffic.presentation.asCurl
import com.fanjv.netproxy.feature.traffic.presentation.displayBody
import com.fanjv.netproxy.feature.traffic.presentation.matchesSession
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TrafficPresentationTest {
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
