package com.fanjv.netproxy.core.module

import com.fanjv.netproxy.core.api.*

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.onEach
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

class ServiceRepositoryTest {
    @Test fun streamDoesNotMergeTrafficFromAnotherCore() = runBlocking {
        val client = ModuleApiClient(object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest) = apiSuccess("{}")
            override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
                consume(ModuleEvent("status", apiSuccess("""{"state":"ready","core_started_at_millis":2000}""").content))
                consume(ModuleEvent("traffic", apiSuccess("""{"available":true,"started_at_millis":1000,"status":{"memory":0,"connections_in":0,"connections_out":0,"uplink_total":999,"downlink_total":0},"process_cpu_ticks":0,"system_cpu_ticks":0,"cpu_count":1}""").content))
                consume(ModuleEvent("traffic", apiSuccess("""{"available":true,"started_at_millis":2000,"status":{"memory":0,"connections_in":0,"connections_out":0,"uplink_total":33,"downlink_total":0},"process_cpu_ticks":0,"system_cpu_ticks":0,"cpu_count":1}""").content))
                awaitCancellation()
            }
        })
        withTimeout(5_000) {
            ServiceRepository(client).observe().onEach { assertTrue(it.getOrThrow().uploadTotal != 999L) }
                .first { it.getOrThrow().uploadTotal == 33L }
        }
        Unit
    }

    @Test fun cancelledRefreshDoesNotPublishLateSuccess() = runBlocking {
        val started = CompletableDeferred<Unit>()
        val complete = CountDownLatch(1)
        val client = ModuleApiClient(transport = TestApiTransport { args ->
            assertEquals("GET", args.method)
            assertEquals("/service", args.path)
            started.complete(Unit)
            check(complete.await(5, TimeUnit.SECONDS))
            ModuleHttpResponse(200, """{"schema":1,"ok":true,"code":"service.status","message":"服务状态","data":{"state":"ready"}}""")
        })
        val repository = ServiceRepository(client)
        var published = false
        var cancelled = false
        val refresh = launch {
            try {
                repository.status()
                published = true
            } catch (error: CancellationException) {
                cancelled = true
                throw error
            }
        }
        try {
            started.await()
            refresh.cancel()
        } finally {
            complete.countDown()
            refresh.cancelAndJoin()
        }
        assertTrue(cancelled)
        assertFalse(published)
    }
}
