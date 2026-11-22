package com.fanjv.netproxy.feature.proxy

import com.fanjv.netproxy.core.api.ApiTestRequest
import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleEvent
import com.fanjv.netproxy.core.api.ModuleHttpResponse
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.feature.proxy.data.PolicyDelayStore
import com.fanjv.netproxy.feature.proxy.data.PolicyDelays
import com.fanjv.netproxy.feature.proxy.data.PolicyGroup
import com.fanjv.netproxy.feature.proxy.data.PolicyMember
import com.fanjv.netproxy.feature.proxy.data.PolicyRepository
import com.fanjv.netproxy.feature.proxy.data.PolicySnapshot
import com.fanjv.netproxy.feature.proxy.data.withRuntime
import com.fanjv.netproxy.feature.proxy.presentation.ManualPolicyTest
import com.fanjv.netproxy.feature.proxy.presentation.PolicyState
import com.fanjv.netproxy.feature.proxy.presentation.PolicyViewModel
import com.fanjv.netproxy.feature.proxy.presentation.displayMembers
import com.fanjv.netproxy.feature.proxy.presentation.displayName
import java.time.Instant
import java.util.concurrent.CopyOnWriteArrayList
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.job
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PolicyViewModelTest {
    @Test fun mainSessionKeepsSelectionCurrentWithoutProxyScreenCollector() = runBlocking {
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        val updates = Channel<ModuleEvent>(Channel.UNLIMITED)
        val transport = object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest) = apiSuccess(snapshot(live = false))

            override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
                for (event in updates) consume(event)
            }
        }
        val vm = PolicyViewModel(PolicyRepository(ModuleApiClient(transport, scope)), scope)
        fun PolicyState.selected() = snapshot?.groups?.first()?.selected.orEmpty()
        try {
            vm.setVisible(true)
            updates.send(groupsEvent())
            withTimeout(5_000) { vm.state.first { it.selected() == "direct" } }

            // With no screen collecting, the main session must still accept a core update.
            updates.send(ModuleEvent("groups", apiSuccess(snapshot(selected = "Example/日本")).content))
            withTimeout(5_000) { vm.state.first { it.selected() == "Example/日本" } }
            assertEquals("Example/日本", vm.state.first().selected())

            updates.send(ModuleEvent("groups", apiSuccess(snapshot(live = false)).content))
            val stopped = withTimeout(5_000) { vm.state.first { it.runtimeConfirmed && it.runtime?.live == false } }
            assertEquals("", stopped.selected())
            assertEquals(2, stopped.snapshot?.groups?.size)
        } finally {
            scope.cancel()
        }
    }

    @Test fun resumeConfirmsSelectionWithoutEventsAndKeepsCardsUntilReadback() = runBlocking {
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        val reads = Channel<CompletableDeferred<ModuleHttpResponse>>(Channel.UNLIMITED)
        val calls = CopyOnWriteArrayList<ApiTestRequest>()
        val transport = object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
                calls += request
                if (request.query["source"] == "config") return apiSuccess(snapshot(live = false))
                val result = CompletableDeferred<ModuleHttpResponse>()
                reads.send(result)
                return result.await()
            }

            override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) = awaitCancellation()
        }
        val vm = PolicyViewModel(PolicyRepository(ModuleApiClient(transport, scope)), scope)
        fun PolicyState.selected() = snapshot?.groups?.first()?.selected.orEmpty()
        try {
            vm.setVisible(true)
            withTimeout(5_000) { reads.receive() }.complete(apiSuccess(snapshot()))
            withTimeout(5_000) { vm.state.first { it.live && it.selected() == "direct" } }
            vm.setVisible(false)
            vm.setVisible(true)
            val resume = withTimeout(5_000) { reads.receive() }
            assertEquals("direct", vm.state.value.selected())
            assertFalse(vm.state.value.live)
            vm.select("Work / 组", "Example/日本")
            assertTrue(calls.none { it.mutation })
            resume.complete(apiSuccess(snapshot(selected = "Example/日本")))
            withTimeout(5_000) { vm.state.first { it.live && it.selected() == "Example/日本" } }

            vm.refresh(showProgress = true)
            withTimeout(5_000) { reads.receive() }.complete(apiSuccess(snapshot(live = false)))
            val stopped = withTimeout(5_000) { vm.state.first { it.runtimeConfirmed && it.runtime?.live == false } }
            assertEquals("", stopped.selected())
            assertFalse(stopped.live)
        } finally {
            scope.cancel()
        }
    }

    @Test fun manualResultMustBeFreshAndBelongToARequestedMember() {
        val previous = PolicyMember("fixture", "socks", delay = 80, delayTime = 200)
        val test = ManualPolicyTest(1234, 200, mapOf(previous.tag to previous))
        assertFalse(test.isNew(previous))
        assertFalse(test.isNew(previous.copy(delayTime = 199, delay = 81)))
        assertFalse(test.isNew(previous.copy(delayTime = 0)))
        assertFalse(test.isNew(previous.copy(delayTime = 201, delay = 0)))
        assertFalse(test.isNew(previous.copy(delayTime = 201, tag = "other")))
        assertTrue(test.isNew(previous.copy(delayTime = 201)))
        assertTrue(test.isNew(previous.copy(delay = 81)))
    }

    @Test fun manualDelaysSurviveReopeningAndAreNotReusedForAnotherCore() = runBlocking {
        var stored: String? = null
        val timestamp = Instant.now().epochSecond
        var live = snapshot(delay = 90, delayTime = timestamp - 1)
        var requests = 0
        val requested = CompletableDeferred<Unit>()
        val acknowledge = CompletableDeferred<Unit>()
        val events = Channel<ModuleEvent>(Channel.UNLIMITED)
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        val transport = object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse = when {
                request.path == "/policy-groups/delay" -> {
                    requests++
                    requested.complete(Unit)
                    acknowledge.await()
                    apiSuccess("{}", "policy.delay_requested")
                }
                request.query["source"] == "config" -> apiSuccess(snapshot(live = false))
                else -> apiSuccess(live)
            }

            override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
                for (event in events) consume(event)
            }
        }
        fun open() = PolicyViewModel(
            PolicyRepository(ModuleApiClient(transport, scope)), scope, PolicyDelayStore({ stored }, { stored = it }),
        ).also { it.setVisible(true) }
        fun PolicyState.delaysByGroup() = snapshot?.groups?.map { group -> group.items.map { it.delay } }
        suspend fun publish(snapshot: String) {
            live = snapshot
            events.send(ModuleEvent("groups", apiSuccess(live).content))
        }
        suspend fun PolicyViewModel.awaitLive() = withTimeout(5_000) { state.first { it.live } }

        try {
            val first = open()
            assertEquals(listOf(listOf(0, 0), listOf(0)), first.awaitLive().delaysByGroup())
            first.delay("Work / 组")
            withTimeout(5_000) { requested.await() }
            publish(snapshot(selected = "Example/日本", delay = 41, delayTime = timestamp + 5))
            withTimeout(5_000) { first.state.first { it.runtime?.groups?.first()?.selected == "Example/日本" } }
            acknowledge.complete(Unit)
            withTimeout(5_000) { first.state.first { !it.operating } }
            assertEquals(listOf(listOf(0, 41), listOf(0)), first.state.value.delaysByGroup())

            // A later frame delivers the second result without overwriting the first one.
            publish(snapshot(delay = 42, delayTime = timestamp + 6, directDelay = 20))
            val measured = listOf(listOf(20, 41), listOf(0))
            withTimeout(5_000) { first.state.first { it.delaysByGroup() == measured } }
            first.setVisible(false)
            first.setVisible(true)
            assertEquals(measured, first.awaitLive().delaysByGroup())
            first.setVisible(false)

            // The replacement ViewModel only sees serialized preferences and a fresh API response.
            val reopened = open()
            assertEquals(measured, reopened.awaitLive().delaysByGroup())
            live = snapshot(delay = 23, delayTime = timestamp + 7, identity = 1235)
            reopened.refresh(showProgress = true)
            val restarted = withTimeout(5_000) {
                reopened.state.first { it.live && it.snapshot?.coreIdentity == 1235L }
            }
            assertEquals(listOf(listOf(0, 0), listOf(0)), restarted.delaysByGroup())
            assertEquals(PolicyDelays(1235), PolicyDelayStore({ stored }, {}).load())
            assertEquals(1, requests)
        } finally {
            scope.cancel()
        }
    }

    @Test fun onlyManualRefreshShowsRefreshIndicatorAndKeepsExistingCards() = runBlocking {
        val initial = CompletableDeferred<Unit>()
        val finish = CompletableDeferred<Unit>()
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        var reads = 0
        val transport = TestApiTransport { request ->
            assertEquals("/policy-groups", request.path)
            assertEquals(mapOf("source" to "config"), request.query)
            assertFalse(request.mutation)
            if (++reads == 1) {
                initial.await()
                apiSuccess(snapshot(live = false))
            } else {
                finish.await()
                apiSuccess(snapshot(selected = "Example/日本", live = false))
            }
        }
        val vm = PolicyViewModel(PolicyRepository(ModuleApiClient(transport, scope)), scope)
        try {
            vm.setVisible(true)
            assertTrue(vm.state.value.loading)
            assertFalse(vm.state.value.refreshing)
            initial.complete(Unit)
            withTimeout(5_000) { vm.state.first { !it.loading } }
            vm.refresh(showProgress = true)
            assertTrue(vm.state.value.refreshing)
            assertEquals("direct", vm.state.value.snapshot?.groups?.first()?.configuredDefault)
            finish.complete(Unit)
            val updated = withTimeout(5_000) { vm.state.first { !it.loading } }
            assertFalse(updated.refreshing)
            assertEquals("Example/日本", updated.snapshot?.groups?.first()?.configuredDefault)
        } finally {
            scope.cancel()
        }
    }

    @Test fun runtimeMetadataCannotAddRemoveOrReorderConfiguredCards() {
        val direct = PolicyMember("direct", "direct")
        val local = PolicyMember("Local/node", "vless")
        val extra = PolicyMember("runtime-only", "socks", 11)
        val configured = PolicyGroup("Work", "selector", false, listOf(direct, local), configuredDefault = "direct")
        val runtime = configured.copy(type = "Selector", selectable = true, items = listOf(extra, direct.copy(delay = 32)),
            selected = extra.tag)
        val view = PolicySnapshot(listOf(configured), false, 0)
            .withRuntime(PolicySnapshot(listOf(runtime, runtime.copy(tag = "Other")), true, 1234))

        val group = view.groups.single()
        assertEquals(listOf(direct.tag, local.tag), group.items.map { it.tag })
        assertEquals(listOf(32, 0), group.items.map { it.delay })
        assertEquals(listOf(true, false), group.items.map { it.runtimeAvailable })
        assertEquals("direct", group.configuredDefault)
        assertEquals("", group.selected)
    }

    @Test fun displayNamesDoNotMergeMembersFromDifferentProviders() {
        val first = PolicyMember("First/日本/01", "vless", name = "日本/01")
        val second = PolicyMember("Second/日本/01", "trojan", name = "日本/01")
        val native = PolicyMember("Work / 组", "selector")
        val group = PolicyGroup("Proxy", "selector", true, listOf(first, second, native))

        assertEquals("日本/01", first.displayName)
        assertEquals("日本/01", second.displayName)
        assertEquals("Work / 组", native.displayName)
        assertEquals(listOf(native.tag, second.tag, first.tag), group.displayMembers(2, live = true).map { it.tag })
    }

    @Test fun delaySortUsesOnlyLivePositiveResults() {
        val original = listOf(PolicyMember("direct", "direct"), PolicyMember("B", "vless", 350),
            PolicyMember("C", "vless", 12), PolicyMember("D", "vless", -1))
        val group = PolicyGroup("Proxy", "selector", true, original)

        assertEquals(original, group.displayMembers(3, live = false))
        assertEquals(listOf("C", "B", "direct", "D"), group.displayMembers(3, live = true).map { it.tag })
    }

    @Test fun selectionRequiresLiveSelectorAndPublishesOnlyConfirmedResult() = runBlocking {
        val calls = CopyOnWriteArrayList<ApiTestRequest>()
        val written = CompletableDeferred<Unit>()
        val confirm = CompletableDeferred<Unit>()
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        val transport = policyTransport { request ->
            calls += request
            when (request.path) {
                "/policy-groups" -> apiSuccess(snapshot(live = false))
                "/policy-groups/select" -> {
                    assertEquals("POST", request.method)
                    assertTrue(request.mutation)
                    assertEquals("1234", request.text("core_started_at_millis"))
                    assertEquals("Work / 组", request.text("group_tag"))
                    assertEquals("Example/日本", request.text("member_tag"))
                    written.complete(Unit)
                    confirm.await()
                    apiSuccess(snapshot(selected = "Example/日本"))
                }
                else -> error("Unexpected request ${request.path}")
            }
        }
        val vm = PolicyViewModel(PolicyRepository(ModuleApiClient(transport, scope)), scope)
        try {
            vm.setVisible(true)
            withTimeout(5_000) { vm.state.first { it.live } }
            vm.select("Auto", "Example/日本")
            vm.select("Work / 组", "missing")
            vm.select("Work / 组", "Example/日本")
            withTimeout(5_000) { written.await() }
            vm.select("Work / 组", "Example/日本")
            assertEquals("direct", vm.state.value.snapshot?.groups?.first()?.selected)
            confirm.complete(Unit)
            withTimeout(5_000) {
                vm.state.first { !it.operating && it.snapshot?.groups?.first()?.selected == "Example/日本" }
            }
            assertEquals(1, calls.count { it.mutation })
            vm.setVisible(false)
            assertFalse(vm.state.value.stale)
            assertFalse(vm.state.value.live)
            assertEquals(listOf("direct", "Example/日本"), vm.state.value.snapshot?.groups?.first()?.items?.map { it.tag })
            vm.setVisible(true)
            assertFalse(vm.state.value.stale)
            assertEquals(2, vm.state.value.snapshot?.groups?.size)
        } finally {
            confirm.complete(Unit)
            scope.cancel()
        }
    }

    @Test fun lostWriteIsNotRepeatedAndDisconnectedSnapshotCannotAuthorizeOperations() = runBlocking {
        val calls = CopyOnWriteArrayList<ApiTestRequest>()
        val listening = CompletableDeferred<Unit>()
        val disconnect = CompletableDeferred<Unit>()
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        val transport = object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
                calls += request
                return if (request.mutation) {
                    ModuleHttpResponse(502, """{"schema":1,"ok":false,"code":"policy.outcome_unknown","message":"Read back before retry","data":null}""")
                } else apiSuccess(snapshot(live = false))
            }

            override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
                consume(groupsEvent())
                listening.complete(Unit)
                disconnect.await()
                if (calls.none { it.mutation }) awaitCancellation()
                error("offline")
            }
        }
        val vm = PolicyViewModel(PolicyRepository(ModuleApiClient(transport, scope)), scope)
        try {
            vm.setVisible(true)
            withTimeout(5_000) {
                listening.await()
                vm.state.first { it.live }
            }
            vm.select("Work / 组", "Example/日本")
            withTimeout(5_000) { vm.state.first { !it.operating && it.noticeIsError && it.notice != UiText.Empty } }
            disconnect.complete(Unit)
            withTimeout(5_000) { vm.state.first { !it.live } }
            assertFalse(vm.state.value.stale)
            assertEquals(2, vm.state.value.snapshot?.groups?.size)
            assertFalse(vm.state.value.live)
            vm.select("Work / 组", "Example/日本")
            vm.delay("Auto")
            assertEquals(1, calls.count { it.mutation })
        } finally {
            scope.cancel()
        }
    }

    @Test fun offlineDefaultAndAsynchronousDelayUseDifferentContracts() = runBlocking {
        val calls = CopyOnWriteArrayList<ApiTestRequest>()
        val coreStarted = CompletableDeferred<Unit>()
        val scope = CoroutineScope(coroutineContext + SupervisorJob(coroutineContext.job))
        val transport = policyTransport(beforeLive = { coreStarted.await() }) { request ->
            calls += request
            when (request.path) {
                "/policy-groups" -> apiSuccess(snapshot(live = false))
                "/policy-groups/delay" -> {
                    assertEquals("Auto", request.text("group_tag"))
                    assertEquals("1234", request.text("core_started_at_millis"))
                    apiSuccess("{}", "policy.delay_requested")
                }
                else -> error("Unexpected request ${request.path}")
            }
        }
        val vm = PolicyViewModel(PolicyRepository(ModuleApiClient(transport, scope)), scope)
        try {
            vm.setVisible(true)
            withTimeout(5_000) { vm.state.first { !it.loading } }
            assertFalse(vm.state.value.live)
            assertEquals("direct", vm.state.value.snapshot?.groups?.first()?.configuredDefault)
            vm.select("Work / 组", "Example/日本")
            vm.delay("Auto")
            assertTrue(calls.none { it.mutation })
            coreStarted.complete(Unit)
            withTimeout(5_000) { vm.state.first { it.live } }
            vm.delay("Auto")
            withTimeout(5_000) { vm.state.first { !it.operating } }
            assertEquals(1, calls.count { it.mutation })
            assertEquals(setOf("Auto"), vm.state.value.manualTests.keys)
            assertEquals(0, vm.state.value.snapshot?.groups?.last()?.items?.single()?.delay)
            vm.setVisible(false)
            assertTrue(vm.state.value.manualTests.isEmpty())
        } finally {
            scope.cancel()
        }
    }

    private fun groupsEvent() = ModuleEvent("groups", apiSuccess(snapshot()).content)

    private fun policyTransport(
        beforeLive: suspend () -> Unit = {},
        block: suspend (ApiTestRequest) -> ModuleHttpResponse,
    ) = object : TestApiTransport() {
        override suspend fun respond(request: ApiTestRequest) = block(request)
        override suspend fun watch(topics: Set<String>, consume: suspend (ModuleEvent) -> Unit) {
            beforeLive()
            consume(groupsEvent())
            awaitCancellation()
        }
    }

    private fun snapshot(
        selected: String = "direct",
        live: Boolean = true,
        delay: Int = 0,
        delayTime: Long = 0,
        directDelay: Int = 0,
        identity: Long = 1234,
    ) = """
        {"live":$live,"core_started_at_millis":${if (live) identity else 0},"groups":[
          {"tag":"Work / 组","type":"selector","selectable":$live,
           ${if (live) "\"selected\":\"$selected\"" else "\"configured_default\":\"$selected\""},
           "items":[{"tag":"direct","type":"direct","url_test_delay":$directDelay,"url_test_time":$delayTime},
                    {"tag":"Example/日本","type":"vless","url_test_delay":$delay,"url_test_time":$delayTime}]},
          {"tag":"Auto","type":"urltest","selectable":false,
           "items":[{"tag":"Example/日本","type":"vless","url_test_delay":$delay,"url_test_time":$delayTime}]}
        ]}
    """.trimIndent()
}
