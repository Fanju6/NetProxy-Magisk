package com.fanjv.netproxy.feature.logs.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import org.junit.Assert.assertEquals
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
    }
}
