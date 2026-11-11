package com.fanjv.netproxy.feature.policy.presentation

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.core.api.ModuleApiException
import com.fanjv.netproxy.core.api.TestApiTransport
import com.fanjv.netproxy.core.api.apiSuccess
import com.fanjv.netproxy.feature.policy.data.PolicyRepository
import com.fanjv.netproxy.feature.policy.model.PolicyModel
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.setMain
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class PolicyViewModelTest {
    @Before fun setMainDispatcher() { Dispatchers.setMain(UnconfinedTestDispatcher()) }
    @After fun resetMainDispatcher() { Dispatchers.resetMain() }

    private val hills = """{"id":"custom-hills","name":"hills","mode":"proxy","package_names":["com.mountains.hills"],"persisted":true}"""
    private val savedHills get() = """{"policy":$hills,"persisted":true,"runtime_synced":false}"""

    @Test fun customSaveWaitsForResponseThenShowsNewCard() = runBlocking {
        val requested = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var posts = 0
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            when (request.method to request.path) {
                "POST" to "/policies" -> {
                    posts++
                    requested.complete(Unit)
                    release.await()
                    apiSuccess(savedHills)
                }
                "GET" to "/policies" -> apiSuccess("""{"policies":[$hills]}""")
                else -> error("Unexpected request: $request")
            }
        })))
        viewModel.beginCustomEdit()
        viewModel.createCustom("hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        withTimeout(5_000) { requested.await() }
        assertEquals(CUSTOM_BUSY_ID, viewModel.state.value.busyId)
        assertFalse(viewModel.state.value.customSaveCompleted)
        assertTrue(viewModel.state.value.policies.isEmpty())
        viewModel.createCustom("hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        release.complete(Unit)
        val state = withTimeout(5_000) { viewModel.state.first { it.customSaveCompleted } }
        assertEquals(1, posts)
        assertEquals("", state.busyId)
        assertEquals("custom-hills", state.savedCustomPolicyId)
        assertEquals("hills", state.policies.single().name)
        assertTrue(state.customSaveError.isEmpty())
        viewModel.beginCustomEdit()
        assertFalse(viewModel.state.value.customSaveCompleted)
    }

    @Test fun customSaveFailureStaysInEditorAndSurvivesSilentRefresh() = runBlocking {
        var reads = 0
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.method == "POST") throw ModuleApiException("usage.invalid", "策略分组没有可用节点")
            run {
                reads++
                apiSuccess("""{"policies":[{"id":"ai","name":"AI","runtime_synced":${reads > 1}}]}""")
            }
        })))
        viewModel.refresh()
        withTimeout(5_000) { viewModel.state.first { !it.loading } }
        viewModel.createCustom("hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        val failed = withTimeout(5_000) { viewModel.state.first { it.customSaveError.isNotEmpty() } }
        assertFalse(failed.customSaveCompleted)
        assertEquals("", failed.busyId)
        assertEquals("策略分组没有可用节点", failed.customSaveError)
        assertEquals("ai", failed.policies.single().id)
        viewModel.refresh(silent = true)
        withTimeout(5_000) { viewModel.state.first { it.policies.single().runtimeSynced } }
        assertEquals(failed.error, viewModel.state.value.error)
        assertEquals(failed.customSaveError, viewModel.state.value.customSaveError)
    }

    @Test fun successfulSaveRetainsConfirmedCardIfRereadFails() = runBlocking {
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.method == "POST") apiSuccess(savedHills) else error("list offline")
        })))
        viewModel.createCustom("hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        val state = withTimeout(5_000) { viewModel.state.first { it.customSaveCompleted } }
        assertEquals("custom-hills", state.policies.single().id)
        assertTrue(state.policies.single().persisted)
        assertEquals("list offline", state.error)
        assertTrue(state.customSaveError.isEmpty())
    }

    @Test fun persistedRuntimeFailureRereadsCardWithoutInvitingDuplicateSave() = runBlocking {
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.method == "POST") throw ModuleApiException(
                "policy.runtime_sync_failed", "策略已保存，但运行时同步失败",
                buildJsonObject { put("persisted", true); put("runtime_synced", false) },
            )
            apiSuccess("""{"policies":[$hills]}""")
        })))
        viewModel.createCustom("hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        val state = withTimeout(5_000) { viewModel.state.first { it.customSaveCompleted } }
        assertEquals("hills", state.policies.single().name)
        assertEquals("策略已保存，但运行时同步失败", state.error)
        assertTrue(state.customSaveError.isEmpty())
        assertFalse(state.policies.single().runtimeSynced)
    }

    @Test fun editingFailureDoesNotCompleteAndRetryUpdatesCard() = runBlocking {
        var failSave = true
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.method == "PUT") {
                if (failSave) error("save offline") else apiSuccess(savedHills)
            } else apiSuccess("""{"policies":[$hills]}""")
        })))
        val policy = PolicyModel("custom-hills", "old name", mode = "proxy")
        viewModel.updateCustom(policy, "hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        withTimeout(5_000) { viewModel.state.first { it.customSaveError.isNotEmpty() } }
        assertFalse(viewModel.state.value.customSaveCompleted)
        failSave = false
        viewModel.updateCustom(policy, "hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        val state = withTimeout(5_000) { viewModel.state.first { it.customSaveCompleted } }
        assertEquals("hills", state.policies.single().name)
        assertTrue(state.customSaveError.isEmpty())
    }

    @Test fun cancelledRefreshCannotOverwriteCustomSave() = runBlocking {
        val firstReadStarted = CompletableDeferred<Unit>()
        var reads = 0
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.method == "POST") apiSuccess(savedHills)
            else {
                reads++
                if (reads == 1) {
                    firstReadStarted.complete(Unit)
                    awaitCancellation()
                }
                apiSuccess("""{"policies":[$hills]}""")
            }
        })))
        viewModel.refresh()
        withTimeout(5_000) { firstReadStarted.await() }
        viewModel.createCustom("hills", listOf("com.mountains.hills"), emptyList(), emptyList())
        val state = withTimeout(5_000) { viewModel.state.first { it.customSaveCompleted } }
        assertEquals("hills", state.policies.single().name)
        assertFalse(state.loading)
        assertTrue(state.error.isEmpty())
    }

    @Test fun loadsPolicyRuntimeStateAndSavesManualSelection() = runBlocking {
        var selectedNode = ""
        val repository = PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            when (request.method to request.path) {
                "GET" to "/policies" -> apiSuccess("""{"policies":[{"id":"ai","name":"AI","group_id":"default","mode":"auto","runtime_synced":true,"groups":[]}]}""")
                "PUT" to "/policies/ai" -> {
                    selectedNode = request.text("node_tag")
                    apiSuccess("""{"persisted":true,"runtime_synced":true}""")
                }
                else -> error("Unexpected request: $request")
            }
        }))
        val viewModel = PolicyViewModel(repository)

        viewModel.refresh()
        withTimeout(5_000) { viewModel.state.first { !it.loading } }
        assertTrue(viewModel.state.value.policies.single().runtimeSynced)
        viewModel.select(viewModel.state.value.policies.single(), "default", "Tokyo")
        withTimeout(5_000) { viewModel.state.first { it.busyId.isEmpty() } }
        assertEquals("Tokyo", selectedNode)
        assertTrue(viewModel.state.value.error.isEmpty())
    }

    @Test fun refreshAndMutationFailuresRemainVisible() = runBlocking {
        var failRead = true
        val repository = PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            if (request.method == "GET" && failRead) error("offline")
            if (request.method == "GET") apiSuccess("""{"policies":[{"id":"ai","name":"AI"}]}""")
            else throw ModuleApiException("policy.runtime_sync_failed", "saved but runtime reload failed")
        }))
        val viewModel = PolicyViewModel(repository)

        viewModel.refresh()
        withTimeout(5_000) { viewModel.state.first { !it.loading } }
        assertEquals("offline", viewModel.state.value.error)
        failRead = false
        viewModel.refresh()
        withTimeout(5_000) { viewModel.state.first { !it.loading && it.policies.isNotEmpty() } }
        viewModel.select(viewModel.state.value.policies.single(), "default", "Tokyo")
        withTimeout(5_000) { viewModel.state.first { it.busyId.isEmpty() && it.error.isNotEmpty() } }
        assertTrue(viewModel.state.value.error.contains("runtime reload failed"))
        assertFalse(viewModel.state.value.loading)
    }

    @Test fun persistedSelectionFailureRefreshesSavedTargetAndKeepsWarning() = runBlocking {
        assertPersistedMutationRefreshes(disable = false)
    }

    @Test fun persistedDisableFailureRefreshesSavedFlagAndKeepsWarning() = runBlocking {
        assertPersistedMutationRefreshes(disable = true)
    }

    @Test fun persistedDeleteFailureRemovesCardAndKeepsWarning() = runBlocking {
        var deleted = false
        val warning = "saved but runtime busy"
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            when {
                request.method == "DELETE" -> {
                    deleted = true
                    throw ModuleApiException("policy.runtime_sync_failed", warning,
                        buildJsonObject { put("persisted", true) })
                }
                else -> apiSuccess("""{"policies":[${if (deleted) "" else hills}]}""")
            }
        })))
        viewModel.refresh()
        withTimeout(5_000) { viewModel.state.first { !it.loading } }
        viewModel.deleteCustom(viewModel.state.value.policies.single())
        withTimeout(5_000) { viewModel.state.first { it.busyId.isEmpty() && it.policies.isEmpty() } }
        assertEquals(warning, viewModel.state.value.error)
    }

    private suspend fun assertPersistedMutationRefreshes(disable: Boolean) {
        var saved = false
        var reads = 0
        val warning = "策略已保存，但运行时同步失败: 已有服务操作正在执行"
        val viewModel = PolicyViewModel(PolicyRepository(ModuleApiClient(TestApiTransport { request ->
            when {
                request.method == "PUT" -> {
                    saved = true
                    throw ModuleApiException("policy.runtime_sync_failed", warning,
                        buildJsonObject { put("persisted", true); put("runtime_synced", false) })
                }
                else -> {
                    reads++
                    apiSuccess("""{"policies":[{"id":"ai","name":"AI","mode":"manual","node_tag":"${if (saved) "Tokyo" else "Old"}","disabled":${saved && disable},"runtime_synced":${!saved}}]}""")
                }
            }
        })))
        viewModel.refresh()
        withTimeout(5_000) { viewModel.state.first { !it.loading } }
        val policy = viewModel.state.value.policies.single()
        if (disable) viewModel.setDisabled(policy, true)
        else viewModel.select(policy, "default", "Tokyo")
        withTimeout(5_000) { viewModel.state.first { it.busyId.isEmpty() && it.policies.single().nodeTag == "Tokyo" } }
        assertEquals(2, reads)
        assertEquals("Tokyo", viewModel.state.value.policies.single().nodeTag)
        assertEquals(disable, viewModel.state.value.policies.single().disabled)
        assertFalse(viewModel.state.value.policies.single().runtimeSynced)
        assertEquals(warning, viewModel.state.value.error)
    }
}
