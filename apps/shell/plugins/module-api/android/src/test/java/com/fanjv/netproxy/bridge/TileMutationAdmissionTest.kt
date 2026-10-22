package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEvent
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.InputStream

@OptIn(ExperimentalCoroutinesApi::class)
class TileMutationAdmissionTest {
    @Test fun tileAdmissionBlocksBridgeWritesButNotReadsOrSubscriptionCancel() = runTest {
        val transport = Transport()
        val dispatcher = StandardTestDispatcher(testScheduler)
        val bridge = ModuleApiBridge(transport, CoroutineScope(SupervisorJob() + dispatcher), workDispatcher = dispatcher)
        bridge.resume()
        val tile = MutationAdmissionGate.process.tryAcquire()!!
        try {
            val calls = listOf<suspend () -> Any>(
                { bridge.serviceAction("service", "start") },
                { bridge.changeMode("mode", "Rule") },
                { bridge.policySelect("""{"requestId":"policy","groupTag":"Proxy","memberTag":"a"}""") },
                { bridge.subscriptionWrite("""{"requestId":"sub","action":"update_all"}""") },
                { bridge.management("""{"requestId":"logs","operation":"logs_clear","input":{"kind":"core"}}""") },
            )
            for (write in calls) {
                assertEquals("api.busy", (runCatching { write() }.exceptionOrNull() as BridgeFailure).code)
            }
            assertTrue(transport.paths.isEmpty())
            bridge.read("read", "service")
            bridge.subscriptionAction("""{"requestId":"cancel","groupId":"sub-a","action":"cancel"}""")
            assertEquals(listOf("/service", "/subscriptions/sub-a/cancel"), transport.paths)
        } finally {
            tile.close()
            bridge.destroy()
        }
    }

    @Test fun bridgeInstancesAndTileShareAdmissionUntilTheActualOperationFinishes() = runTest {
        val transport = Transport().apply { barrier = CompletableDeferred() }
        val dispatcher = StandardTestDispatcher(testScheduler)
        val scope = CoroutineScope(SupervisorJob() + dispatcher)
        val first = ModuleApiBridge(transport, scope, workDispatcher = dispatcher)
        val second = ModuleApiBridge(transport, scope, workDispatcher = dispatcher)
        first.resume()
        second.resume()
        try {
            val write = async { first.serviceAction("first", "start") }
            runCurrent()
            assertNull(MutationAdmissionGate.process.tryAcquire())
            assertEquals("api.busy", (runCatching { second.serviceAction("second", "stop") }
                .exceptionOrNull() as BridgeFailure).code)
            transport.barrier!!.complete(Unit)
            write.await()
            MutationAdmissionGate.process.tryAcquire()!!.close()
            second.serviceAction("second", "stop")
            assertEquals(listOf("/service/start", "/service/stop"), transport.paths)
        } finally {
            transport.barrier?.complete(Unit)
            first.destroy()
            second.destroy()
        }
    }

    private class Transport : ModuleApiTransport {
        val paths = mutableListOf<String>()
        var barrier: CompletableDeferred<Unit>? = null
        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>,
            mutation: Boolean, decode: (Int, InputStream) -> T): T {
            paths.add(path)
            if (path == "/service/start") barrier?.await()
            return decode(200, """{"schema":1,"ok":true,"code":"service.status","message":"fixture","data":{}}""".byteInputStream())
        }
        override suspend fun reconnect(address: String): Unit = error("No pairing")
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("No files")
        override suspend fun image(path: String): ByteArray = error("No images")
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit): Unit = error("No SSE")
    }
}
