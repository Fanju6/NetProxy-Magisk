package com.fanjv.netproxy.core.api

import kotlinx.coroutines.*
import kotlinx.coroutines.flow.first
import kotlinx.serialization.json.Json
import org.junit.Assert.*
import org.junit.Test
import java.util.concurrent.atomic.AtomicInteger
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository

class ModuleApiClientTest {
    @Test fun explicitReconnectConfirmsThePinnedServiceBeforeSuccess() = runBlocking {
        val steps = mutableListOf<String>()
        val client = ModuleApiClient(object : TestApiTransport() {
            override suspend fun reconnect(address: String) { steps += address }
            override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
                steps += request.method + request.path
                assertFalse(request.mutation)
                return apiSuccess("{}", "api.info")
            }
        })
        client.reconnect("https://127.0.0.1:9092")
        assertEquals(listOf("https://127.0.0.1:9092", "GET/info"), steps)
    }

    @Test fun subscriptionListAndStreamUseTheSameSummaryContract() = runBlocking {
        val content = """[{"id":"sample","name":"Example","type":"subscription","active":true}]"""
        val repository = SubscriptionRepository(ModuleApiClient(object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
                assertEquals("GET", request.method)
                assertEquals("/subscriptions", request.path)
                return apiSuccess(content)
            }
            override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
                assertEquals(setOf("subscriptions"), topics)
                consume(ModuleEvent("subscriptions", apiSuccess(content).content))
                awaitCancellation()
            }
        }))
        withTimeout(5_000) { assertEquals(repository.list(), repository.observe().first().getOrThrow()) }
    }

    private val codec = ModuleApiCodec(Json)

    @Test fun responseTypesAndHTTPStatusAreStrict() {
        for (content in listOf("[]", "null", "{}",
            """{"schema":"1","ok":true,"code":"ok","message":""}""",
            """{"schema":{},"ok":true,"code":"ok","message":""}""",
            """{"schema":1,"ok":"true","code":"ok","message":""}""",
            """{"schema":1,"ok":{},"code":"ok","message":""}""",
            """{"schema":1,"ok":true,"code":1,"message":""}""",
            """{"schema":1,"ok":true,"message":""}""")) {
            assertThrows(ModuleApiException::class.java) { codec.decode(200, content) }
        }
        val content = """{"schema":1,"ok":false,"code":"config.conflict","message":"changed","data":{"persisted":true}}"""
        val error = assertThrows(ModuleApiException::class.java) { codec.decode(409, content) }
        assertEquals("config.conflict", error.resultCode)
        assertTrue(error.persisted)
        assertThrows(ModuleApiException::class.java) { codec.decode(503, apiSuccess("{}").content) }
    }

    @Test fun connectionOnlyAllowsPinnedNumericLoopback() {
        val secret = "a".repeat(64)
        for (address in listOf("https://127.0.0.1:9091", "https://[::1]:9091")) {
            ModuleConnection(address, secret, secret).validate()
        }
        for (address in listOf("http://127.0.0.1:9091", "https://localhost:9091", "https://192.168.1.1:9091",
            "https://127.0.0.1:9091/", "https://127.0.0.1:9091?secret=x", "https://user@127.0.0.1:9091")) {
            assertThrows(IllegalArgumentException::class.java) { ModuleConnection(address, secret, secret).validate() }
        }
    }

    @Test fun acceptedWriteFinishesButReadCancels() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val completed = CompletableDeferred<Unit>()
        val client = ModuleApiClient(TestApiTransport {
            entered.complete(Unit)
            release.await()
            completed.complete(Unit)
            apiSuccess("{}")
        })
        val write = launch { client.post("/service/stop") }
        entered.await()
        write.cancel()
        release.complete(Unit)
        withTimeout(5_000) { write.join(); completed.await() }
        val readEntered = CompletableDeferred<Unit>()
        val readClient = ModuleApiClient(TestApiTransport { readEntered.complete(Unit); awaitCancellation() })
        val read = launch { readClient.read<com.fanjv.netproxy.core.module.ServiceStatusSnapshot>("/service") }
        readEntered.await()
        withTimeout(5_000) { read.cancelAndJoin() }
    }

    @Test fun consumersShareOneStreamAndLastConsumerClosesIt() = runBlocking {
        val active = AtomicInteger()
        val peak = AtomicInteger()
        val client = ModuleApiClient(object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest) = apiSuccess("{}")
            override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
                peak.updateAndGet { maxOf(it, active.incrementAndGet()) }
                try {
                    topics.forEach { consume(ModuleEvent(it, apiSuccess("{}").content)) }
                    awaitCancellation()
                } finally { active.decrementAndGet() }
            }
        })
        val first = async { client.observe(setOf("status")).first().getOrThrow() }
        val second = async { client.observe(setOf("groups")).first().getOrThrow() }
        withTimeout(5_000) {
            assertTrue(first.await().containsKey("status"))
            assertTrue(second.await().containsKey("groups"))
            while (active.get() != 0) delay(10)
        }
        assertEquals(1, peak.get())
    }
}
