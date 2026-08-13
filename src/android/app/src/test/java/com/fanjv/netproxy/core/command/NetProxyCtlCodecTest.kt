package com.fanjv.netproxy.core.command

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
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
                    stdout = listOf(
                        """{"schema":1,"ok":false,"code":"subscription.runtime_sync_failed","message":"更新失败","data":{"persisted":true,"runtime_synced":false,"runtime_sync_state":"failed","runtime_sync_pending":true,"original_code":"subscription.download_failed","cause":"fixture failure"}}"""
                    ),
                    stderr = listOf("details")
                )
            )
        }

        assertEquals("subscription.runtime_sync_failed", error.resultCode)
        assertEquals("更新失败", error.message)
        val data = error.data.jsonObject
        assertEquals("true", data["persisted"]?.jsonPrimitive?.content)
        assertEquals("false", data["runtime_synced"]?.jsonPrimitive?.content)
        assertEquals("failed", data["runtime_sync_state"]?.jsonPrimitive?.content)
        assertEquals("true", data["runtime_sync_pending"]?.jsonPrimitive?.content)
        assertEquals("subscription.download_failed", data["original_code"]?.jsonPrimitive?.content)
        assertEquals("fixture failure", data["cause"]?.jsonPrimitive?.content)
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

    @Test
    fun `availability accepts structured command errors`() = runBlocking {
        val client = NetProxyCtlClient(
            transport = NetProxyCtlTransport { _, _ ->
                NetProxyCtlOutput(
                    successful = false,
                    stdout = listOf(
                        "{\"schema\":1,\"ok\":false,\"code\":\"command.failed\",\"message\":\"配置无效\",\"data\":{}}"
                    ),
                    stderr = emptyList()
                )
            }
        )

        assertTrue(client.isAvailable())
    }

    @Test
    fun `availability rejects missing or invalid cli output`() = runBlocking {
        val client = NetProxyCtlClient(
            transport = NetProxyCtlTransport { _, _ ->
                NetProxyCtlOutput(
                    successful = false,
                    stdout = emptyList(),
                    stderr = listOf("命令不存在")
                )
            }
        )

        assertFalse(client.isAvailable())
    }

    @Test
    fun `availability keeps incompatible cli distinguishable from missing cli`() = runBlocking {
        val client = NetProxyCtlClient(
            transport = NetProxyCtlTransport { _, _ ->
                NetProxyCtlOutput(
                    successful = true,
                    stdout = listOf(
                        "{\"schema\":2,\"ok\":true,\"code\":\"service.status\",\"message\":\"\",\"data\":{}}"
                    ),
                    stderr = emptyList()
                )
            }
        )

        assertTrue(client.isAvailable())
    }

    @Test
    fun `subscription mutations rely on the subscription download timeout`() = runBlocking {
        val observed = mutableListOf<Long>()
        val client = NetProxyCtlClient(
            transport = NetProxyCtlTransport { _, timeoutMillis ->
                observed += timeoutMillis
                NetProxyCtlOutput(
                    successful = true,
                    stdout = listOf("{\"schema\":1,\"ok\":true,\"code\":\"ok\",\"message\":\"\",\"data\":{}}"),
                    stderr = emptyList()
                )
            }
        )

        client.execute("sub", "edit", "fixture")
        client.execute("sub", "update-all")
        client.execute("sub", "list")
        client.execute("service", "start")

        assertEquals(listOf(0L, 0L, 30_000L, 120_000L), observed)
    }
}
