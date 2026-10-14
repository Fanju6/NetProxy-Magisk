package com.fanjv.netproxy.core.module

import kotlinx.coroutines.*
import kotlinx.coroutines.flow.first
import org.junit.Assert.*
import org.junit.Test

class ModuleAccessViewModelTest {
    @Test fun explicitReconnectCancelsOldProbeBeforePublishingConfirmedAccess(): Unit = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val cancelled = CompletableDeferred<Unit>()
        val connected = CompletableDeferred<Unit>()
        val vm = ModuleAccessViewModel(object : ModuleEnvironment {
            override val totalMemoryBytes = 0L
            override suspend fun availability(): ModuleAvailability {
                entered.complete(Unit)
                try { awaitCancellation() } finally { cancelled.complete(Unit) }
            }
            override suspend fun reconnect(address: String) {
                assertEquals("https://127.0.0.1:9092", address)
                assertTrue(cancelled.isCompleted)
                connected.complete(Unit)
            }
        }, this)
        vm.refresh()
        withTimeout(5_000) { entered.await(); vm.reconnect("https://127.0.0.1:9092"); connected.await() }
        assertTrue(vm.state.value!!.available)
    }

    private fun model(scope: CoroutineScope, query: suspend () -> ModuleAvailability) =
        ModuleAccessViewModel(object : ModuleEnvironment {
            override val totalMemoryBytes = 0L
            override suspend fun availability() = query()
            override suspend fun reconnect(address: String) { query() }
        }, scope)

    @Test fun refreshIsSingleFlightAndKeepsPreviousStateWhileChecking() = runBlocking {
        val scope = CoroutineScope(coroutineContext + SupervisorJob())
        val started = CompletableDeferred<Unit>()
        val finish = CompletableDeferred<Unit>()
        var calls = 0
        try {
            val vm = model(scope) {
                if (++calls == 1) ModuleAvailability(ModuleConnectionState.UNAVAILABLE)
                else {
                    started.complete(Unit)
                    finish.await()
                    ModuleAvailability(ModuleConnectionState.READY)
                }
            }
            assertNull(vm.state.value)
            vm.refresh()
            withTimeout(2_000) { vm.state.first { it != null } }
            assertFalse(vm.state.value!!.available)
            vm.refresh()
            started.await()
            vm.refresh()
            assertFalse(vm.state.value!!.available)
            finish.complete(Unit)
            withTimeout(2_000) { vm.state.first { it?.available == true } }
            assertEquals(2, calls)
        } finally { scope.cancel() }
    }

    @Test fun failedProbeCannotLeaveCheckingOrKeepStaleAccess(): Unit = runBlocking {
        val scope = CoroutineScope(coroutineContext + SupervisorJob())
        var fail = false
        try {
            val vm = model(scope) {
                if (fail) error("unavailable") else ModuleAvailability(ModuleConnectionState.READY)
            }
            vm.refresh()
            withTimeout(2_000) { vm.state.first { it?.available == true } }
            fail = true
            vm.refresh()
            withTimeout(2_000) { vm.state.first { it?.available == false } }
        } finally { scope.cancel() }
    }

    @Test fun cancellationIsNotACompletedDenial() = runBlocking {
        val scope = CoroutineScope(coroutineContext + SupervisorJob())
        val vm = model(scope) { throw CancellationException() }
        vm.refresh()
        scope.coroutineContext.job.children.toList().joinAll()
        assertNull(vm.state.value)
        scope.cancel()
    }
}
