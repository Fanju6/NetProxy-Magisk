package com.fanjv.netproxy.feature.logs.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class LogParserTest {
    @Test
    fun parseNativeUsesStructuredComponentEventAndResult() {
        val entries = Json.parseToJsonElement(
            """[{"timestamp":"2026-08-13 12:00:00","level":"WARN","component":"subscription","event":"subscription.edit","result":"persisted","error_code":"subscription.convert_failed","message":"订阅编辑，但后续操作失败：订阅下载、转换或校验失败"}]"""
        ).jsonArray

        val item = LogParser.parseNative(entries).single()

        assertEquals(LogLevel.WARN, item.level)
        assertEquals("subscription", item.tag)
        assertEquals("subscription.edit", item.event)
        assertEquals("persisted", item.result)
        assertEquals("subscription.convert_failed", item.errorCode)
        assertEquals("订阅编辑，但后续操作失败：订阅下载、转换或校验失败", item.message)
        assertNull(item.connectionId)
        assertNull(item.latency)
        assertNull(item.latencyMs)
    }

    @Test
    fun parseKernelKeepsSingBoxSpecificFlowParsing() {
        val item = LogParser.parseKernel(
            "2026-08-13T12:00:00Z INFO routing: routed connection from 10.0.0.2:1234 to example.com:443 [Proxy]"
        ).single()

        assertEquals("routing", item.tag)
        assertEquals("10.0.0.2:1234", item.outboundFlow?.source)
        assertEquals("example.com:443", item.outboundFlow?.target)
        assertEquals("Proxy", item.outboundFlow?.outbound)
        assertEquals("", item.event)
        assertEquals("", item.result)
        assertEquals("", item.errorCode)
        assertNull(item.connectionId)
        assertNull(item.latency)
        assertNull(item.latencyMs)
    }

    @Test
    fun parseKernelExtractsTagConnectionOnceAcrossTimestampFormats() {
        val lines = listOf(
            "2026-10-08T12:00:00Z INFO [12345 12.5ms] routing: connected" to "10-08 12:00:00",
            "+0800 2026-10-08 12:00:00 INFO [12345 12.5ms] routing: connected" to "10-08 12:00:00",
            "INFO[0000] [12345 12.5ms] routing: connected" to "",
        )

        lines.forEach { (line, timestamp) ->
            val item = LogParser.parseKernel(line).single()

            assertEquals(line, item.rawLine)
            assertEquals(timestamp, item.timestamp)
            assertEquals(LogLevel.INFO, item.level)
            assertEquals("routing", item.tag)
            assertEquals("connected", item.message)
            assertEquals("12345", item.connectionId)
            assertEquals("12.5ms", item.latency)
            assertEquals(12, item.latencyMs)
        }
    }

    @Test
    fun parseKernelRetainsShortFormatConnectionWithoutLatency() {
        val line = "INFO[0000] [12345] outbound/direct[direct]: outbound connection to example.com:443"

        val item = LogParser.parseKernel(line).single()

        assertEquals(line, item.rawLine)
        assertEquals("outbound/direct[direct]", item.tag)
        assertEquals("12345", item.connectionId)
        assertNull(item.latency)
        assertNull(item.latencyMs)
        assertEquals(OutboundFlow("Local", "example.com:443", "direct"), item.outboundFlow)
    }

    @Test
    fun parseKernelCleansMessageConnectionBeforeDetailFlowParsing() {
        val prefixes = listOf(
            "2026-10-08T12:00:00Z INFO",
            "+0800 2026-10-08 12:00:00 INFO",
            "INFO[0000]",
        )

        prefixes.forEach { prefix ->
            val line = "$prefix outbound/direct[direct]: [12345 1.5s] outbound packet connection to example.com:443"
            val item = LogParser.parseKernel(line).single()

            assertEquals(line, item.rawLine)
            assertEquals("outbound/direct[direct]", item.tag)
            assertEquals("outbound packet connection to example.com:443", item.message)
            assertEquals("12345", item.connectionId)
            assertEquals("1.5s", item.latency)
            assertEquals(1500, item.latencyMs)
            assertEquals(OutboundFlow("Local", "example.com:443", "direct"), item.outboundFlow)
        }
    }

    @Test
    fun parseKernelKeepsRoutedFlowWithConnectionMetadata() {
        val line = "INFO[0000] [12345 800ms] routing: routed connection from 10.0.0.2:1234 to example.com:443 [Proxy]"

        val item = LogParser.parseKernel(line).single()

        assertEquals(line, item.rawLine)
        assertEquals("routing", item.tag)
        assertEquals("12345", item.connectionId)
        assertEquals(800, item.latencyMs)
        assertEquals(OutboundFlow("10.0.0.2:1234", "example.com:443", "Proxy"), item.outboundFlow)
    }

    @Test
    fun parseKernelPrefersTagConnectionOverMessagePrefix() {
        val item = LogParser.parseKernel(
            "INFO[0000] [12345 2ms] routing: [67890 3s] connected"
        ).single()

        assertEquals("routing", item.tag)
        assertEquals("12345", item.connectionId)
        assertEquals("2ms", item.latency)
        assertEquals(2, item.latencyMs)
        assertEquals("[67890 3s] connected", item.message)
    }

    @Test
    fun parseKernelConvertsLatencyWithoutChangingDisplayedText() {
        val durations = listOf(
            "0ms" to 0,
            "0.5ms" to 0,
            "799ms" to 799,
            "800ms" to 800,
            "1499ms" to 1499,
            "1500ms" to 1500,
            "1.5s" to 1500,
            "12.5MS" to 12,
            "2S" to 2000,
            "999999999999999999999s" to Int.MAX_VALUE,
        )

        durations.forEach { (duration, milliseconds) ->
            val item = LogParser.parseKernel("INFO[0000] [12345 $duration] routing: connected").single()

            assertEquals(duration, item.latency)
            assertEquals(milliseconds, item.latencyMs)
        }
    }

    @Test
    fun parseKernelKeepsInvalidLatencyUnknownInsteadOfReportingZero() {
        listOf("unknown", "ms", "1.2.3ms", "-1ms", "garbage1ms", "1m2s").forEach { duration ->
            val item = LogParser.parseKernel("INFO[0000] [12345 $duration] routing: connected").single()

            assertEquals("12345", item.connectionId)
            assertEquals(duration, item.latency)
            assertNull(item.latencyMs)
        }
    }

    @Test
    fun parseKernelLeavesNonConnectionBracketsUntouched() {
        listOf("[not-an-id 2ms] connected", "[12345 2ms extra] connected", "connected [12345 2ms]").forEach { message ->
            val item = LogParser.parseKernel("INFO[0000] routing: $message").single()

            assertEquals(message, item.message)
            assertNull(item.connectionId)
            assertNull(item.latency)
            assertNull(item.latencyMs)
        }
    }

    @Test
    fun parseNativeDoesNotGuessKernelMetadataFromMessage() {
        val entries = Json.parseToJsonElement(
            """[{"timestamp":"2026-10-08 12:00:00","level":"INFO","component":"service","event":"service.start","result":"success","error_code":"","message":"[12345 1.5s] connected"}]"""
        ).jsonArray

        val item = LogParser.parseNative(entries).single()

        assertEquals("service", item.tag)
        assertEquals("[12345 1.5s] connected", item.message)
        assertEquals(
            "[2026-10-08 12:00:00] [INFO] [service] [service.start] [success] [-] [12345 1.5s] connected",
            item.rawLine,
        )
        assertNull(item.connectionId)
        assertNull(item.latency)
        assertNull(item.latencyMs)
    }

    @Test
    fun parseKernelKeepsLargeLogOrderAndLastLineWhileSkippingBlankLines() {
        val lines = (0 until 800).map { index -> "INFO[0000] routing: line $index" }

        val items = LogParser.parseKernel("\n${lines.joinToString("\n\n")}\n")

        assertEquals(800, items.size)
        assertEquals(lines, items.map(LogItem::rawLine))
        assertEquals("line 799", items.last().message)
    }

    @Test
    fun parseKernelKeepsUnknownFormatReadable() {
        val line = "unstructured kernel output"

        val item = LogParser.parseKernel(line).single()

        assertEquals(line, item.rawLine)
        assertEquals(line, item.message)
        assertEquals("Kernel", item.tag)
        assertEquals(LogLevel.UNKNOWN, item.level)
        assertNull(item.connectionId)
        assertNull(item.latency)
        assertNull(item.latencyMs)
    }
}
