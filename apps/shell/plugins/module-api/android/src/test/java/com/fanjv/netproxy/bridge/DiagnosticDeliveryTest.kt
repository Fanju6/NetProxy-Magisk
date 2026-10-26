package com.fanjv.netproxy.bridge

import com.fanjv.netproxy.core.api.ModuleApiTransport
import com.fanjv.netproxy.core.api.ModuleEvent
import com.fanjv.netproxy.core.api.ModuleApiException
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import kotlinx.serialization.json.*
import org.junit.After
import org.junit.Assert.*
import org.junit.Test
import java.io.File
import java.io.InputStream

@OptIn(ExperimentalCoroutinesApi::class)
class DiagnosticDeliveryTest {
    private val bridges = mutableListOf<ModuleApiBridge>()
    @After fun close() = bridges.forEach { it.destroy() }

    @Test fun oversizedDownloadRejectsOnceWithTypedStatusAndNeverPreparesDelivery() = runTest {
        val bridge = bridge { throw ModuleApiException("api.body_too_large", "private fixture") }
        var replies = 0
        bridge.dispatch({ bridge.management(REQUEST) }) { result ->
            replies++
            val error = result.exceptionOrNull() as BridgeFailure
            assertEquals("api.body_too_large", error.code)
            assertEquals(413, error.status)
            assertFalse(error.message.orEmpty().contains("private fixture"))
        }
        runCurrent()
        assertEquals(1, replies)
    }

    @Test fun ipcResolvesBeforeChooserPauseAndNeverEmitsASecondFailure() = runTest {
        val order = mutableListOf<String>()
        var cleanups = 0
        lateinit var bridge: ModuleApiBridge
        bridge = bridge {
            DiagnosticShare.Prepared({ order.add("chooser"); bridge.pause() }, { cleanups++ })
        }
        bridge.dispatch({ bridge.management(REQUEST) }) { result ->
            val response = result.getOrThrow()
            assertEquals("diagnostics.prepared", response.body["code"]!!.jsonPrimitive.content)
            assertEquals(Json.parseToJsonElement("""{"prepared":true}"""), response.body["data"])
            assertFalse(response.body.toString().contains("path"))
            order.add("resolved")
        }
        runCurrent()
        assertEquals(listOf("resolved", "chooser"), order)
        assertEquals(0, cleanups)
        bridge.resume(); runCurrent()
        assertEquals(listOf("resolved", "chooser"), order)
    }

    @Test fun failedIpcDeliveryDiscardsThePreparedFileWithoutLaunchingOrRetrying() = runTest {
        var attempts = 0
        var launches = 0
        var cleanups = 0
        val bridge = bridge { DiagnosticShare.Prepared({ launches++ }, { cleanups++ }) }
        bridge.dispatch({ bridge.management(REQUEST) }) {
            attempts++
            throw IllegalStateException("Callback released")
        }
        runCurrent()
        assertEquals(1, attempts)
        assertEquals(0, launches)
        assertEquals(1, cleanups)
    }

    @Test fun documentOrActivityInvalidationDuringDeliveryCannotOpenAStaleChooser() = runTest {
        for (boundary in listOf("pause", "destroy", "document")) {
            var launches = 0
            var cleanups = 0
            var replies = 0
            val bridge = bridge { DiagnosticShare.Prepared({ launches++ }, { cleanups++ }) }
            val originalView = Any()
            bridge.load(originalView, true)
            bridge.dispatch({ bridge.management(REQUEST) }) { result ->
                assertTrue(result.isSuccess)
                replies++
                when (boundary) {
                    "pause" -> bridge.pause()
                    "destroy" -> bridge.destroy()
                    else -> bridge.load(Any(), true)
                }
            }
            runCurrent()
            assertEquals(1, replies)
            assertEquals(0, launches)
            assertEquals(1, cleanups)
        }
    }

    @Test fun cancellationAfterPreparationBeforeMainDeliveryCleansUpOnce() = runTest {
        var launches = 0
        var cleanups = 0
        var replies = 0
        val bridge = bridge { DiagnosticShare.Prepared({ launches++ }, { cleanups++ }) }
        bridge.dispatch({
            bridge.management(REQUEST).also { bridge.pause(); bridge.resume() }
        }) { result ->
            replies++
            assertTrue(result.isFailure)
        }
        runCurrent()
        assertEquals(1, replies)
        assertEquals(0, launches)
        assertEquals(1, cleanups)
    }

    @Test fun failedLaunchCleansUpWithoutRejectingTheAlreadyPreparedReply() = runTest {
        var replies = 0
        var launches = 0
        var cleanups = 0
        val bridge = bridge {
            DiagnosticShare.Prepared({ launches++; throw IllegalStateException("No receiving Activity") }, { cleanups++ })
        }
        bridge.dispatch({ bridge.management(REQUEST) }) { result -> replies++; assertTrue(result.isSuccess) }
        runCurrent()
        assertEquals(1, replies)
        assertEquals(1, launches)
        assertEquals(1, cleanups)
    }

    @Test fun preparedHandoffIsOneShotAndDoesNotDeleteFilesAlreadyGrantedToTheChooser() {
        var launches = 0
        var cleanups = 0
        val handedOff = DiagnosticShare.Prepared({ launches++ }, { cleanups++ })
        handedOff.deliver(); handedOff.deliver(); handedOff.discard()
        assertEquals(1, launches)
        assertEquals(0, cleanups)
        val abandoned = DiagnosticShare.Prepared({ launches++ }, { cleanups++ })
        abandoned.discard(); abandoned.discard(); abandoned.deliver()
        assertEquals(1, launches)
        assertEquals(1, cleanups)
    }

    private fun TestScope.bridge(prepare: suspend () -> DiagnosticShare.Prepared) = ModuleApiBridge(
        NoNetwork(), CoroutineScope(SupervisorJob() + StandardTestDispatcher(testScheduler)),
        workDispatcher = StandardTestDispatcher(testScheduler), prepareDiagnostics = prepare,
    ).also { bridges.add(it); it.resume() }

    private class NoNetwork : ModuleApiTransport {
        override suspend fun <T> request(method: String, path: String, body: JsonObject?, query: Map<String, String>, mutation: Boolean, decode: (Int, InputStream) -> T): T = error("No HTTP from handoff")
        override suspend fun download(path: String, query: Map<String, String>, destination: File): Unit = error("No path IPC")
        override suspend fun image(path: String): ByteArray = error("No image IPC")
        override suspend fun reconnect(address: String): Unit = error("No reconnect")
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit): Unit = error("No event IPC")
    }

    companion object {
        private const val REQUEST = """{"requestId":"diagnostic","operation":"diagnostics"}"""
    }
}
