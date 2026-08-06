package com.fanjv.netproxy.core.command

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class NetProxyCtlCodecTest {
    private val codec = NetProxyCtlCodec(Json)

    @Test
    fun `decodes a successful response`() {
        val response = codec.decode(
            NetProxyCtlOutput(
                successful = true,
                stdout = listOf("{\"schema\":1,\"ok\":true,\"code\":\"node.listed\",\"message\":\"\",\"data\":{\"count\":2}}"),
                stderr = emptyList()
            )
        )

        assertEquals("node.listed", response.code)
        assertEquals(2, response.data.jsonObject["count"]?.jsonPrimitive?.content?.toInt())
    }

    @Test
    fun `rejects additional stdout around the response`() {
        val error = assertThrows(NetProxyCtlException::class.java) {
            codec.decode(
                NetProxyCtlOutput(
                    successful = true,
                    stdout = listOf(
                        "unexpected log",
                        "{\"schema\":1,\"ok\":true,\"code\":\"ok\",\"message\":\"\",\"data\":{}}"
                    ),
                    stderr = emptyList()
                )
            )
        }

        assertEquals("transport.invalid_json", error.resultCode)
    }

    @Test
    fun `keeps the structured command error`() {
        val error = assertThrows(NetProxyCtlException::class.java) {
            codec.decode(
                NetProxyCtlOutput(
                    successful = false,
                    stdout = listOf("{\"schema\":1,\"ok\":false,\"code\":\"subscription.failed\",\"message\":\"更新失败\",\"data\":{}}"),
                    stderr = listOf("details")
                )
            )
        }

        assertEquals("subscription.failed", error.resultCode)
        assertEquals("更新失败", error.message)
    }

    @Test
    fun `rejects unsupported schemas`() {
        val error = assertThrows(NetProxyCtlException::class.java) {
            codec.decode(
                NetProxyCtlOutput(
                    successful = true,
                    stdout = listOf("{\"schema\":2,\"ok\":true,\"code\":\"ok\",\"message\":\"\",\"data\":{}}"),
                    stderr = emptyList()
                )
            )
        }

        assertEquals("transport.unsupported_schema", error.resultCode)
    }
}
