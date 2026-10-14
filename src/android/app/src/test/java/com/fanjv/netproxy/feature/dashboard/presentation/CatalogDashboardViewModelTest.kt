package com.fanjv.netproxy.feature.dashboard.presentation

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.core.module.ModuleAvailability
import com.fanjv.netproxy.core.module.ModuleConnectionState
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.ui.UiText
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.job
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.yield
import org.junit.Assert.*
import org.junit.Test

class CatalogDashboardViewModelTest {
    private class Transport(val handler: suspend (ApiTestRequest) -> String) : TestApiTransport() {
        val calls = CopyOnWriteArrayList<ApiTestRequest>()
        override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
            calls += request
            val data = handler(request)
            return apiSuccess(data)
        }
    }

    private fun snapshot(mode: String, state: String = "stopped", total: Long = 0) =
        """{"state":"$state","outbound_mode":"$mode","available_outbound_modes":["Rule","Direct"],"download_total":$total}"""

    private fun model(scope: CoroutineScope, transport: Transport) = CatalogDashboardViewModel(
        ServiceRepository(ModuleApiClient(transport = transport)),
        1_000L, scope,
    ).apply { setAvailability(ModuleAvailability(ModuleConnectionState.READY)) }

    private suspend fun CatalogDashboardViewModel.available() = withTimeout(5_000) {
        state.first { it.connected }
    }

    private suspend fun settle() {
        val jobs = currentCoroutineContext().job.children.toList()
        withTimeout(5_000) { jobs.forEach { it.join() } }
    }

    @Test fun availabilityLossBlocksNewReadsButDoesNotCancelStartedControl() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val transport = Transport { args ->
            if (args.path == "/mode") {
                entered.complete(Unit)
                release.await()
                "{}"
            } else snapshot("Rule")
        }
        val vm = model(this, transport)
        vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.loading } }
        vm.setMode("Direct")
        withTimeout(5_000) { entered.await() }
        vm.setAvailability(ModuleAvailability(ModuleConnectionState.UNAVAILABLE))
        vm.refresh()
        vm.toggleService()
        release.complete(Unit)
        withTimeout(5_000) { vm.state.first { it.operation.isEmpty() } }
        settle()
        assertFalse(vm.state.value.connected)
        assertEquals(1, transport.calls.count { it.path == "/mode" })
        assertEquals(1, transport.calls.count { it.path == "/service" })
        assertFalse(transport.calls.any { it.path == "/service/start" })
    }

    @Test fun latestRefreshWinsAndStaleFailureCannotMarkServiceFailed() = runBlocking {
        val count = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val finished = CompletableDeferred<Unit>()
        val vm = model(this, Transport {
            if (count.incrementAndGet() == 1) {
                entered.complete(Unit)
                withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                finished.complete(Unit)
                throw IllegalStateException("stale failure")
            } else snapshot("Direct")
        })
        vm.available(); vm.refresh()
        withTimeout(5_000) { entered.await() }
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.outboundMode == "Direct" } }
        release.complete(Unit)
        withTimeout(5_000) { finished.await() }
        settle()
        assertEquals("stopped", vm.state.value.serviceState)
        assertEquals("", vm.state.value.serviceError)
    }

    @Test fun modeChangesRunInOrderAndInvalidateOldRefresh() = runBlocking {
        val count = AtomicInteger()
        var savedMode = "Rule"
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val modeEntered = CompletableDeferred<Unit>()
        val modeRelease = CompletableDeferred<Unit>()
        val transport = Transport { args ->
            if (args.path == "/mode") {
                modeEntered.complete(Unit)
                withTimeout(10_000) { modeRelease.await() }
                savedMode = args.text("mode")
                "{}"
            } else when (count.incrementAndGet()) {
                1 -> snapshot("Rule")
                2 -> {
                    entered.complete(Unit)
                    withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                    snapshot("Rule", "ready", 999)
                }
                else -> snapshot(savedMode)
            }
        }
        val vm = model(this, transport)
        vm.available(); vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.loading } }
        vm.refresh()
        withTimeout(5_000) { entered.await() }
        vm.setMode("Direct")
        vm.setMode("Rule")
        vm.toggleService()
        vm.refresh()
        withTimeout(5_000) { modeEntered.await() }
        release.complete(Unit)
        yield()
        assertEquals("mode", vm.state.value.operation)
        assertEquals(0L, vm.state.value.downloadTotal)
        assertEquals(1, transport.calls.count { it.path == "/mode" })
        modeRelease.complete(Unit)
        settle()
        withTimeout(5_000) { vm.state.first { it.outboundMode == "Rule" && it.operation.isEmpty() } }
        assertEquals(listOf("Direct", "Rule"), transport.calls.filter { it.path == "/mode" }.map { it.text("mode") })
        assertFalse(transport.calls.any { it.path in setOf("/service/start", "/service/stop") })
    }

    @Test fun serviceStartAndStopInvalidateOldSnapshot() = runBlocking {
        for (initial in listOf("stopped", "ready")) {
            val action = if (initial == "stopped") "start" else "stop"
            val final = if (initial == "stopped") "ready" else "stopped"
            val count = AtomicInteger()
            var savedMode = "Rule"
            val entered = CompletableDeferred<Unit>()
            val release = CompletableDeferred<Unit>()
            val actionRelease = CompletableDeferred<Unit>()
            val transport = Transport { args ->
                if (args.path == "/service/$action") {
                    withTimeout(10_000) { actionRelease.await() }
                    "{}"
                } else if (args.path == "/mode") {
                    savedMode = args.text("mode")
                    "{}"
                }
                else when (count.incrementAndGet()) {
                    1 -> snapshot("Rule", initial)
                    2 -> {
                        entered.complete(Unit)
                        withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                        snapshot("Rule", initial)
                    }
                    else -> snapshot(savedMode, final)
                }
            }
            val vm = model(this, transport)
            vm.available(); vm.refresh()
            withTimeout(5_000) { vm.state.first { !it.loading } }
            vm.refresh()
            withTimeout(5_000) { entered.await() }
            vm.toggleService()
            assertEquals(action, vm.state.value.operation)
            assertEquals("Rule", vm.state.value.outboundMode)
            vm.setMode("Direct")
            assertFalse(transport.calls.any { it.path == "/mode" })
            actionRelease.complete(Unit)
            withTimeout(5_000) { vm.state.first {
                it.serviceState == final && it.outboundMode == "Direct" && it.operation.isEmpty()
            } }
            release.complete(Unit)
            settle()
            assertEquals(final, vm.state.value.serviceState)
            assertEquals(1, transport.calls.count { it.path == "/mode" })
        }
    }

    @Test fun hidingDashboardInvalidatesInFlightPollingSnapshot() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val vm = model(this, Transport {
            entered.complete(Unit)
            withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
            snapshot("Direct")
        })
        vm.available()
        vm.setVisible(true)
        withTimeout(5_000) { entered.await() }
        vm.setVisible(false)
        release.complete(Unit)
        settle()
        assertEquals("unknown", vm.state.value.outboundMode)
        assertEquals("", vm.state.value.serviceError)
    }

    @Test fun cancelledModeOperationDoesNotPublishFailureNotice() = runBlocking {
        val transport = Transport { args ->
            if (args.path == "/mode") throw CancellationException("page closed")
            snapshot("Rule")
        }
        val vm = model(this, transport)
        vm.available(); vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.loading } }
        vm.setMode("Direct")
        settle()
        assertEquals(UiText.Empty, vm.state.value.notice)
        assertEquals(0L, vm.state.value.noticeId)
        assertEquals("", vm.state.value.serviceError)
    }

    @Test fun cancelledPageCannotPublishLateOperationResult() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        lateinit var vm: CatalogDashboardViewModel
        coroutineScope {
            val worker = launch {
                vm = model(this, Transport { args ->
                    if (args.path == "/service/start") {
                        entered.complete(Unit)
                        withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                        "{}"
                    } else snapshot("Rule")
                })
                vm.available(); vm.refresh()
                withTimeout(5_000) { vm.state.first { !it.loading } }
                vm.toggleService()
            }
            withTimeout(5_000) { entered.await() }
            worker.cancel()
            release.complete(Unit)
            worker.join()
        }
        assertEquals("starting", vm.state.value.serviceState)
        assertEquals(UiText.Empty, vm.state.value.notice)
        assertEquals(0L, vm.state.value.noticeId)
    }
}
