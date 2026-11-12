package com.fanjv.netproxy.feature.proxy

import com.fanjv.netproxy.core.api.ApiTestRequest
import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleEvent
import com.fanjv.netproxy.core.api.ModuleHttpResponse
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.core.ui.UiText
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
import java.util.concurrent.CopyOnWriteArrayList
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.job
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PolicyViewModelTest {
    @Test fun latencyIsHiddenUntilTheSameGroupAndCoreHaveANewerManualResult() {
        val node = PolicyMember("fixture", "socks")
        val group = PolicyGroup("Work", "selector", false, listOf(node))
        val other = group.copy(tag = "Other")
        val previous = node.copy(delay = 80, delayTime = 200)
        val configured = PolicySnapshot(listOf(group, other), live = false, coreIdentity = 0)
        val initial = PolicySnapshot(
            listOf(group.copy(items = listOf(previous)), other.copy(items = listOf(previous))),
            live = true,
            coreIdentity = 1234,
        )
        val page = PolicyState(configured = configured, runtime = initial)
        fun delays(state: PolicyState) = state.snapshot!!.groups.map { it.items.single().delay }

        assertEquals(listOf(0, 0), delays(page))
        val test = ManualPolicyTest(1234, 200, mapOf(node.tag to previous))
        val testing = page.copy(manualTests = mapOf(group.tag to test))
        assertEquals(listOf(0, 0), delays(testing))
        val updated = initial.copy(groups = initial.groups.map { it.copy(items = listOf(previous.copy(delayTime = 201))) })
        assertEquals(listOf(80, 0), delays(testing.copy(runtime = updated)))
        assertEquals(listOf(0, 0), delays(testing.copy(runtime = updated.copy(coreIdentity = 1235))))
        val untimed = initial.copy(groups = initial.groups.map { it.copy(items = listOf(previous.copy(delayTime = 0))) })
        assertEquals(listOf(0, 0), delays(testing.copy(runtime = untimed)))
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
                vm.state.first { it.pendingGroup == null && it.snapshot?.groups?.first()?.selected == "Example/日本" }
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
            withTimeout(5_000) { vm.state.first { it.pendingGroup == null && it.error != UiText.Empty } }
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
            withTimeout(5_000) { vm.state.first { it.pendingGroup == null } }
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

    private fun snapshot(selected: String = "direct", live: Boolean = true) = """
        {"live":$live,"core_started_at_millis":${if (live) 1234 else 0},"groups":[
          {"tag":"Work / 组","type":"selector","selectable":$live,
           ${if (live) "\"selected\":\"$selected\"" else "\"configured_default\":\"$selected\""},
           "items":[{"tag":"direct","type":"direct"},{"tag":"Example/日本","type":"vless"}]},
          {"tag":"Auto","type":"urltest","selectable":false,"items":[{"tag":"Example/日本","type":"vless"}]}
        ]}
    """.trimIndent()
}
