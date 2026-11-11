package com.fanjv.netproxy.feature.catalog.presentation.nodes

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
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
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class CatalogNodesViewModelTest {
    @get:Rule val folder = TemporaryFolder()

    private class Transport(val handler: suspend (ApiTestRequest) -> String) : TestApiTransport() {
        val calls = CopyOnWriteArrayList<ApiTestRequest>()
        override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
            calls += request
            val data = handler(request)
            return apiSuccess(data)
        }
    }

    private fun snapshot(tag: String) =
        """{"selection":{"active_group_id":"default","selector_mode":"manual","selected":"default/$tag","runtime_selected":"Local/$tag"}}"""

    private fun model(scope: CoroutineScope, transport: Transport) = CatalogNodesViewModel(
        NodeRepository(ModuleApiClient(transport = transport)),
        importNode = { error("Unexpected import") }, scope = scope,
    )

    private suspend fun settle() {
        val jobs = currentCoroutineContext().job.children.toList()
        withTimeout(5_000) { jobs.forEach { it.join() } }
    }

    @Test fun consumingFailureNoticeDoesNotTurnReadFailureIntoEmptyNodes() = runBlocking {
        var fail = true
        val vm = model(this, Transport { if (fail) error("cannot read") else snapshot("new") })
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.loadFailed } }
        vm.clearNotice()
        assertEquals(UiText.Empty, vm.state.value.error)
        assertTrue(vm.state.value.loadFailed)
        fail = false
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.selection.selected == "default/new" } }
        assertFalse(vm.state.value.loadFailed)
        settle()
    }

    @Test fun latestRefreshWinsEvenWhenOldRequestFinishesLast() = runBlocking {
        val count = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val finished = CompletableDeferred<Unit>()
        val transport = Transport {
            if (count.incrementAndGet() == 1) {
                entered.complete(Unit)
                withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                finished.complete(Unit)
                snapshot("old")
            } else snapshot("new")
        }
        val vm = model(this, transport)
        vm.refresh()
        withTimeout(5_000) { entered.await() }
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.selection.selected == "default/new" } }
        release.complete(Unit)
        withTimeout(5_000) { finished.await() }
        settle()
        assertEquals("default/new", vm.state.value.selection.selected)
        assertEquals(UiText.Empty, vm.state.value.error)
    }

    @Test fun nodeSelectionInvalidatesOldRefreshAndReadsConfirmedRuntimeSelection() = runBlocking {
        val count = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val transport = Transport { args ->
            if (args.path == "/selection") "{}"
            else if (count.incrementAndGet() == 1) {
                entered.complete(Unit)
                withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                snapshot("old")
            } else snapshot("chosen")
        }
        val vm = model(this, transport)
        vm.refresh()
        withTimeout(5_000) { entered.await() }
        vm.useNode("default", "chosen")
        assertEquals("select", vm.state.value.operation)
        vm.useNode("default", "duplicate")
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.selection.runtimeSelected == "Local/chosen" } }
        release.complete(Unit)
        settle()
        assertEquals("default/chosen", vm.state.value.selection.selected)
        assertEquals(2, count.get())
        assertEquals(1, transport.calls.count { it.path == "/selection" })
    }

    @Test fun autoSelectionKeepsResolvedNodeUntilConfirmedSnapshotReplacesIt() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var reads = 0
        val initial = """{"selection":{"active_group_id":"default","selector_mode":"urltest","selected":"Auto/default","active_group_runtime_tag":"Local","runtime_selected":"Local/fast"}}"""
        val transport = Transport { args ->
            if (args.path == "/selection") "{}"
            else if (++reads == 1) initial
            else {
                entered.complete(Unit)
                withTimeout(5_000) { release.await() }
                initial.replace("Local/fast", "Local/faster")
            }
        }
        val vm = model(this, transport)
        vm.refresh(); settle()
        vm.useAuto("default")
        withTimeout(5_000) { entered.await() }
        assertEquals("Local/fast", vm.state.value.selection.runtimeSelected)
        assertEquals("Local", vm.state.value.selection.activeGroupRuntimeTag)
        release.complete(Unit); settle()
        assertEquals("Local/faster", vm.state.value.selection.runtimeSelected)
        assertEquals(2, reads)
    }

    @Test fun regionSelectionReadsConfirmedRegionAndAutoClearsIt() = runBlocking {
        var region = ""
        val transport = Transport { request ->
            if (request.path == "/selection") {
                region = if ("region" in request.body) request.text("region") else ""
                assertEquals("default", request.text("group_id"))
                assertEquals("", request.text("node_tag"))
                "{}"
            } else {
                """{"selection":{"active_group_id":"default","selector_mode":"urltest","active_region":"$region","active_group_runtime_tag":"Local","runtime_selected":"Local/Tokyo"}}"""
            }
        }
        val vm = model(this, transport)
        vm.useRegion("default", "jp"); settle()
        assertEquals("jp", vm.state.value.selection.region)
        assertEquals("Local/Tokyo", vm.state.value.selection.runtimeSelected)
        vm.useAuto("default"); settle()
        assertEquals("", vm.state.value.selection.region)
    }

    @Test fun regionDelayPreservesDisambiguatedRuntimePrefix() = runBlocking {
        val transport = Transport { request ->
            if (request.path == "/nodes/delay") {
                assertEquals("group-1", request.text("group_id"))
                assertEquals("jp", request.text("region"))
                """{"target":"Auto/Shared [group-1] [jp]","groups":[{"tag":"Auto/Shared [group-1] [jp]","items":[{"tag":"Shared [group-1]/Tokyo","url_test_delay":88}]}]}"""
            } else {
                """{"groups":[{"group":{"id":"group-1","name":"Shared","type":"local","runtime_tag":"Shared [group-1]"},"nodes":[{"tag":"Tokyo","region":"jp"},{"tag":"Seattle","region":"us"}]}]}"""
            }
        }
        val vm = model(this, transport)
        vm.refresh(); settle()
        vm.testRegionDelay("group-1", "jp"); settle()
        assertEquals("88", vm.state.value.latencies["group-1/Tokyo"])
        assertEquals("88", vm.state.value.latencies["Auto/group-1 [jp]"])
        assertFalse(vm.state.value.latencies.containsKey("group-1/Seattle"))
        assertEquals(UiText.Empty, vm.state.value.error)
    }

    @Test fun cancelledSelectionDoesNotPublishPageFailure() = runBlocking {
        val transport = Transport { throw CancellationException("page closed") }
        val vm = model(this, transport)
        vm.useAuto("default")
        settle()
        assertEquals(UiText.Empty, vm.state.value.error)
        assertEquals(0L, vm.state.value.noticeId)
        assertFalse(transport.calls.any { it.path == "/nodes/snapshot" })
    }

    @Test fun cancelledPageCannotApplyLateSnapshotOrFailure() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        lateinit var vm: CatalogNodesViewModel
        coroutineScope {
            val worker = launch {
                vm = model(this, Transport {
                    entered.complete(Unit)
                    withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                    throw IllegalStateException("late failure")
                })
                vm.refresh()
            }
            withTimeout(5_000) { entered.await() }
            worker.cancel()
            release.complete(Unit)
            worker.join()
        }
        assertEquals(UiText.Empty, vm.state.value.error)
        assertEquals(0L, vm.state.value.noticeId)
    }
}
