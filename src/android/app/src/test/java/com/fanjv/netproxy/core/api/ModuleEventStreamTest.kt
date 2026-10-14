package com.fanjv.netproxy.core.api

import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import com.fanjv.netproxy.core.module.ServiceRepository
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.*
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ModuleEventStreamTest {
    private class Session(val topics: Set<String>, val events: Channel<ModuleEvent> = Channel(Channel.UNLIMITED))

    private class Streams : TestApiTransport() {
        val sessions = mutableListOf<Session>()
        var active = 0
        var peak = 0
        override suspend fun respond(request: ApiTestRequest) = apiSuccess("{}")
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
            val session = Session(topics)
            sessions += session
            active++
            peak = maxOf(peak, active)
            try {
                for (event in session.events) consume(event)
            } finally { active-- }
        }
        fun send(name: String, data: String = "{}") {
            check(sessions.last().events.trySend(ModuleEvent(name, apiSuccess(data).content)).isSuccess)
        }
    }

    @Test fun removalAndBriefTabAbsenceDoNotReconnectOrShrinkTopics() = runTest {
        val transport = Streams()
        val client = ModuleApiClient(transport, backgroundScope)
        val first = backgroundScope.launch { client.observe(setOf("status")).collect() }
        runCurrent()
        val second = backgroundScope.launch { client.observe(setOf("groups")).collect() }
        runCurrent()
        assertEquals(setOf("status", "groups"), transport.sessions.last().topics)
        val count = transport.sessions.size
        second.cancelAndJoin()
        runCurrent()
        assertEquals(count, transport.sessions.size)
        first.cancelAndJoin()
        advanceTimeBy(1_999)
        runCurrent()
        assertEquals(1, transport.active)
        val returned = backgroundScope.launch { client.observe(setOf("status")).collect() }
        runCurrent()
        assertEquals(count, transport.sessions.size)
        assertEquals(setOf("status", "groups"), transport.sessions.last().topics)
        returned.cancelAndJoin()
        advanceTimeBy(2_000)
        runCurrent()
        assertEquals(0, transport.active)
        assertEquals(1, transport.peak)
    }

    @Test fun realStopClearsUnionAndSnapshotBeforeTheNextForegroundSession() = runTest {
        val transport = Streams()
        val client = ModuleApiClient(transport, backgroundScope)
        val received = mutableListOf<Result<Map<String, ModuleEventFrame>>>()
        val initial = backgroundScope.launch { client.observe(setOf("status", "groups")).collect { received += it } }
        runCurrent()
        transport.send("status")
        transport.send("groups")
        runCurrent()
        assertEquals(1, received.size)
        initial.cancelAndJoin()
        advanceTimeBy(2_000)
        runCurrent()
        received.clear()
        backgroundScope.launch { client.observe(setOf("status")).collect { received += it } }
        runCurrent()
        assertEquals(setOf("status"), transport.sessions.last().topics)
        assertTrue(received.isEmpty())
        transport.send("status")
        runCurrent()
        assertEquals(1, received.size)
    }

    @Test fun disconnectDropsOldFramesAndReconnectWaitsForCompleteSnapshot() = runTest {
        val transport = Streams()
        val client = ModuleApiClient(transport, backgroundScope)
        val received = mutableListOf<Result<Map<String, ModuleEventFrame>>>()
        backgroundScope.launch { client.observe(setOf("status", "groups")).collect { received += it } }
        runCurrent()
        transport.send("status")
        transport.send("groups")
        runCurrent()
        val oldGroup = received.single().getOrThrow().getValue("groups")
        transport.sessions.last().events.close(java.io.IOException("fixture"))
        runCurrent()
        assertEquals("api.disconnected", (received.last().exceptionOrNull() as ModuleApiException).resultCode)
        advanceTimeBy(1_000)
        runCurrent()
        val count = received.size
        transport.send("status")
        runCurrent()
        assertEquals(count, received.size)
        transport.send("groups")
        runCurrent()
        assertNotSame(oldGroup, received.last().getOrThrow().getValue("groups"))
        assertEquals(1, transport.peak)
    }

    @Test fun trafficDoesNotReemitTheSameGroupFrame() = runTest {
        val transport = Streams()
        val client = ModuleApiClient(transport, backgroundScope)
        val groups = mutableListOf<ModuleEventFrame>()
        backgroundScope.launch { client.observe(setOf("status", "traffic")).collect() }
        backgroundScope.launch { client.observe(setOf("groups")).collect { groups += it.getOrThrow().getValue("groups") } }
        runCurrent()
        transport.send("groups")
        transport.send("status")
        runCurrent()
        repeat(20) {
            transport.send("traffic", """{"available":false,"sample":$it}""")
            runCurrent()
        }
        assertEquals(1, groups.size)
        transport.send("groups")
        runCurrent()
        assertEquals(2, groups.size)
    }

    @Test fun typedStreamSchemaFailureIsDeliveredToEveryConsumer() = runTest {
        val transport = Streams()
        val repository = SubscriptionRepository(ModuleApiClient(transport, backgroundScope))
        val errors = mutableListOf<Throwable>()
        repeat(2) {
            backgroundScope.launch { repository.observe().collect { errors += it.exceptionOrNull()!! } }
        }
        runCurrent()
        transport.sessions.last().events.send(ModuleEvent("subscriptions",
            """{"schema":2,"ok":true,"code":"ok","message":"","data":[]}"""))
        runCurrent()
        assertEquals(2, errors.size)
        assertEquals("api.unsupported_schema", (errors.first() as ModuleApiException).resultCode)
        assertSame(errors.first(), errors.last())
        assertEquals(1, transport.sessions.size)
    }

    @Test fun unavailableTrafficKeepsServiceStatusButIncompleteAvailableTrafficFails() = runTest {
        val transport = Streams()
        val repository = ServiceRepository(ModuleApiClient(transport, backgroundScope))
        val received = mutableListOf<Result<com.fanjv.netproxy.core.module.ServiceStatusSnapshot>>()
        backgroundScope.launch { repository.observe().collect { received += it } }
        runCurrent()
        transport.send("status", """{"state":"ready","core_started_at_millis":2000,"upload_total":33}""")
        transport.send("traffic", """{"available":false}""")
        runCurrent()
        assertEquals(33L, received.last().getOrThrow().uploadTotal)
        transport.send("traffic", """{"available":true,"started_at_millis":2000}""")
        runCurrent()
        assertEquals("api.invalid_response", (received.last().exceptionOrNull() as ModuleApiException).resultCode)
    }
}
