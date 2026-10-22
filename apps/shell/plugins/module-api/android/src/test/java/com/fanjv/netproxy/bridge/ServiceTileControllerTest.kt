package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEvent
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.IOException
import java.io.InputStream

@OptIn(ExperimentalCoroutinesApi::class)
class ServiceTileControllerTest {
    @Test fun knownStatesUseFreshReadOneWriteAndAuthoritativeReadback() = runTest {
        for ((before, action, after) in listOf(
            Triple(ServiceTileState.STOPPED, "start", ServiceTileState.READY),
            Triple(ServiceTileState.READY, "stop", ServiceTileState.STOPPED),
        )) {
            val api = Api(before, after)
            val shown = mutableListOf<ServiceTileState>()
            val controller = ServiceTileController(api, shown::add, MutationAdmissionGate())
            assertEquals(ServiceTileClick.DONE, controller.click())
            assertEquals(listOf("read", action, "read"), api.calls)
            assertEquals(listOf(ServiceTileState.CHECKING, before, ServiceTileState.WORKING, after), shown)
        }
    }

    @Test fun refreshIsNotAToggleCacheAndNeverStartsAWriteOrPollingLoop() = runTest {
        val api = Api(ServiceTileState.READY, ServiceTileState.STOPPED, ServiceTileState.STARTING)
        val shown = mutableListOf<ServiceTileState>()
        val controller = ServiceTileController(api, shown::add, MutationAdmissionGate())
        controller.refresh()
        assertEquals(listOf("read"), api.calls)
        assertEquals(ServiceTileState.READY, shown.last())
        controller.click()
        assertEquals(listOf("read", "read", "start", "read"), api.calls)
        assertEquals(ServiceTileState.STARTING, shown.last())
        runCurrent()
        assertEquals(4, api.calls.size)
    }

    @Test fun transitionalStatesAreReadOnlyAndFailedOrUnknownOpenTheApp() = runTest {
        for (state in listOf(ServiceTileState.PREPARING, ServiceTileState.STARTING, ServiceTileState.STOPPING,
            ServiceTileState.FAILED, ServiceTileState.UNKNOWN)) {
            val api = Api(state)
            val shown = mutableListOf<ServiceTileState>()
            val controller = ServiceTileController(api, shown::add, MutationAdmissionGate())
            val expected = if (state == ServiceTileState.FAILED || state == ServiceTileState.UNKNOWN)
                ServiceTileClick.OPEN_APP else ServiceTileClick.DONE
            assertEquals(expected, controller.click())
            assertEquals(listOf("read"), api.calls)
            assertEquals(state, shown.last())
        }
    }

    @Test fun readFailureNeverFallsBackToTheDisplayedTileState() = runTest {
        val api = Api(ServiceTileState.STOPPED).apply { readFailure = IOException("private fixture") }
        val shown = mutableListOf<ServiceTileState>()
        val gate = MutationAdmissionGate()
        assertEquals(ServiceTileClick.OPEN_APP, ServiceTileController(api, shown::add, gate).click())
        assertEquals(listOf("read"), api.calls)
        assertEquals(ServiceTileState.UNKNOWN, shown.last())
        gate.tryAcquire()!!.close()
    }

    @Test fun failedOrAmbiguousWritesAreNeverReplayedAndStillReadBack() = runTest {
        for (after in listOf(ServiceTileState.READY, ServiceTileState.STOPPED, ServiceTileState.UNKNOWN)) {
            val api = Api(ServiceTileState.STOPPED, after).apply { writeFailure = IOException("private fixture") }
            val shown = mutableListOf<ServiceTileState>()
            assertEquals(ServiceTileClick.OPEN_APP,
                ServiceTileController(api, shown::add, MutationAdmissionGate()).click())
            assertEquals(listOf("read", "start", "read"), api.calls)
            assertEquals(after, shown.last())
        }
    }

    @Test fun postWriteReadFailureDoesNotReportTheRequestedStateAsSuccess() = runTest {
        val api = Api(ServiceTileState.STOPPED).apply { failReadAfterWrite = true }
        val shown = mutableListOf<ServiceTileState>()
        assertEquals(ServiceTileClick.OPEN_APP,
            ServiceTileController(api, shown::add, MutationAdmissionGate()).click())
        assertEquals(listOf("read", "start", "read"), api.calls)
        assertEquals(ServiceTileState.UNKNOWN, shown.last())
    }

    @Test fun rapidClicksAndRefreshesDoNotQueueExtraWritesOrOverwriteReadback() = runTest {
        val finishWrite = CompletableDeferred<Unit>()
        val api = Api(ServiceTileState.STOPPED, ServiceTileState.READY).apply { writeBarrier = finishWrite }
        val shown = mutableListOf<ServiceTileState>()
        val gate = MutationAdmissionGate()
        val controller = ServiceTileController(api, shown::add, gate)
        val first = async { controller.click() }
        runCurrent()
        repeat(10) {
            assertEquals(ServiceTileClick.DONE, controller.click())
            controller.refresh()
        }
        assertNull(gate.tryAcquire())
        assertEquals(listOf("read", "start"), api.calls)
        finishWrite.complete(Unit)
        first.await()
        assertEquals(ServiceTileState.READY, shown.last())
        assertEquals(listOf("read", "start", "read"), api.calls)
        gate.tryAcquire()!!.close()
    }

    @Test fun processAdmissionIsHeldAcrossPreflightAndReadbackAndBusyIsNotQueued() = runTest {
        val gate = MutationAdmissionGate()
        val owner = gate.tryAcquire()!!
        val api = Api(ServiceTileState.STOPPED)
        val shown = mutableListOf<ServiceTileState>()
        assertEquals(ServiceTileClick.DONE, ServiceTileController(api, shown::add, gate).click())
        assertEquals(listOf(ServiceTileState.BUSY), shown)
        assertTrue(api.calls.isEmpty())
        owner.close()
        val next = gate.tryAcquire()!!
        owner.close()
        assertNull(gate.tryAcquire())
        next.close()

        val preflight = CompletableDeferred<Unit>()
        val readback = CompletableDeferred<Unit>()
        api.states.add(ServiceTileState.READY)
        api.readBarriers.addAll(listOf(preflight, readback))
        val work = async { ServiceTileController(api, shown::add, gate).click() }
        runCurrent()
        assertNull(gate.tryAcquire())
        assertEquals(listOf("read"), api.calls)
        preflight.complete(Unit)
        runCurrent()
        assertNull(gate.tryAcquire())
        assertEquals(listOf("read", "start", "read"), api.calls)
        readback.complete(Unit)
        work.await()
        gate.tryAcquire()!!.close()
    }

    @Test fun closingServiceDuringWriteDoesNotCancelTheWriteOrReleaseItsGateEarly() = runTest {
        val barrier = CompletableDeferred<Unit>()
        val gate = MutationAdmissionGate()
        val api = Api(ServiceTileState.STOPPED, ServiceTileState.READY).apply { writeBarrier = barrier }
        val controller = ServiceTileController(api, {}, gate)
        val work = async { controller.click() }
        runCurrent()
        work.cancel()
        runCurrent()
        assertFalse(work.isCompleted)
        assertNull(gate.tryAcquire())
        barrier.complete(Unit)
        runCurrent()
        assertEquals(listOf("read", "start", "read"), api.calls)
        assertTrue(work.isCompleted)
        gate.tryAcquire()!!.close()
    }

    @Test fun cancellationBeforeWritePropagatesAndReleasesAdmission() = runTest {
        val gate = MutationAdmissionGate()
        val api = Api(ServiceTileState.STOPPED).apply { readFailure = CancellationException() }
        assertTrue(runCatching { ServiceTileController(api, {}, gate).click() }.exceptionOrNull() is CancellationException)
        assertEquals(listOf("read"), api.calls)
        gate.tryAcquire()!!.close()
    }

    @Test fun finishingIsBoundedAndTimeoutDoesNotLeaveTheTileInWorkingState() = runTest {
        val gate = MutationAdmissionGate()
        val api = Api(ServiceTileState.STOPPED).apply { writeBarrier = CompletableDeferred() }
        val shown = mutableListOf<ServiceTileState>()
        assertEquals(ServiceTileClick.OPEN_APP, ServiceTileController(api, shown::add, gate).click())
        assertEquals(ServiceTileState.UNKNOWN, shown.last())
        assertEquals(listOf("read", "start"), api.calls)
        gate.tryAcquire()!!.close()
    }

    @Test fun fixedHttpAdapterValidatesEnvelopesAndNeverTrustsActionStatus() = runTest {
        val transport = Transport()
        val api = HttpServiceTileApi(transport)
        for (wire in listOf("stopped", "preparing", "starting", "ready", "stopping", "failed", "future")) {
            transport.content = envelope("""{"state":"$wire"}""")
            assertEquals(ServiceTileState.fromWire(wire), api.read())
        }
        for (data in listOf("{}", """{"state":null}""", """{"state":true}""", """{"state":4}""")) {
            transport.content = envelope(data)
            assertEquals(ServiceTileState.UNKNOWN, api.read())
        }
        transport.content = envelope("""{"status":{"state":"ready"}}""")
        api.start()
        api.stop()
        assertEquals(listOf("POST /service/start", "POST /service/stop"), transport.calls.takeLast(2))
        for ((status, content) in listOf(
            200 to "malformed", 200 to envelope("{}").replace("\"schema\":1", "\"schema\":7"),
            200 to envelope("{}").replace("\"ok\":true", "\"ok\":\"true\""),
            200 to envelope("{}").replace("\"ok\":true", "\"ok\":false"),
            401 to envelope("{}"), 503 to envelope("{}"),
            200 to envelope("""{"state":"${"x".repeat(4 * 1024 * 1024)}"}"""),
        )) {
            transport.status = status
            transport.content = content
            assertTrue(runCatching { api.read() }.isFailure)
        }
    }

    private class Api(vararg initial: ServiceTileState) : ServiceTileApi {
        val states = ArrayDeque(initial.toList())
        val calls = mutableListOf<String>()
        var readFailure: Exception? = null
        var writeFailure: Exception? = null
        var writeBarrier: CompletableDeferred<Unit>? = null
        var failReadAfterWrite = false
        val readBarriers = ArrayDeque<CompletableDeferred<Unit>>()

        override suspend fun read(): ServiceTileState {
            calls.add("read")
            readBarriers.removeFirstOrNull()?.await()
            readFailure?.let { throw it }
            if (failReadAfterWrite && "start" in calls) throw IOException("private fixture")
            return states.removeFirst()
        }

        override suspend fun start() = write("start")
        override suspend fun stop() = write("stop")
        private suspend fun write(action: String) {
            calls.add(action)
            writeBarrier?.await()
            writeFailure?.let { throw it }
        }
    }

    private class Transport : ModuleApiTransport {
        var status = 200
        var content = envelope("{}")
        val calls = mutableListOf<String>()
        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>,
            mutation: Boolean, decode: (Int, InputStream) -> T): T {
            assertTrue(query.isEmpty())
            if (path == "/service") {
                assertEquals("GET", method)
                assertFalse(mutation)
                assertNull(body)
            } else {
                assertTrue(path in setOf("/service/start", "/service/stop"))
                assertEquals("POST", method)
                assertTrue(mutation)
                assertEquals(JsonObject(emptyMap()), body)
            }
            calls.add("$method $path")
            return decode(status, content.byteInputStream())
        }
        override suspend fun reconnect(address: String): Unit = error("No pairing")
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("No files")
        override suspend fun image(path: String): ByteArray = error("No images")
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit): Unit = error("No polling or SSE")
    }

    companion object {
        private fun envelope(data: String) =
            """{"schema":1,"ok":true,"code":"service.status","message":"fixture","data":$data}"""
    }
}
