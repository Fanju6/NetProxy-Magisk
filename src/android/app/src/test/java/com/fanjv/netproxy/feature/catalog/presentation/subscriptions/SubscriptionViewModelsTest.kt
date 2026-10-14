package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import com.fanjv.netproxy.core.api.*

import com.fanjv.netproxy.core.ui.UiText
import com.fanjv.netproxy.feature.catalog.data.NodeRepository
import com.fanjv.netproxy.feature.catalog.data.SubscriptionRepository
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class SubscriptionViewModelsTest {
    @get:Rule val folder = TemporaryFolder()
    private val calls = mutableListOf<ApiTestRequest>()

    private fun catalog(action: suspend (ApiTestRequest) -> ModuleHttpResponse): ModuleApiClient =
        ModuleApiClient(transport = TestApiTransport { args ->
            calls += args
            action(args)
        })

    private fun output(data: String) = apiSuccess(data)

    @Test fun consumingFailureNoticeDoesNotTurnReadFailureIntoEmptySubscriptions() = runBlocking {
        var fail = true
        val vm = SubscriptionsViewModel(SubscriptionRepository(catalog {
            if (fail) error("cannot read") else output("[]")
        }), this)
        vm.refresh()
        withTimeout(5_000) { vm.state.first { it.loadFailed } }
        vm.clearNotice()
        assertEquals(UiText.Empty, vm.state.value.error)
        assertTrue(vm.state.value.loadFailed)
        fail = false
        vm.refresh()
        withTimeout(5_000) { vm.state.first { !it.loading && !it.loadFailed } }
        assertTrue(vm.state.value.groups.isEmpty())
    }

    private fun editor(id: String, url: String, name: String = "Sample") = output(
        """{"id":"$id","name":"$name","url":"$url"}"""
    )

    private fun persisted(id: String): Nothing = throw ModuleApiException(
        "subscription.runtime_sync_failed", "persisted failure",
        JsonObject(mapOf("group_id" to JsonPrimitive(id), "persisted" to JsonPrimitive(true), "runtime_sync_pending" to JsonPrimitive(true)))
    )

    private suspend fun SubscriptionEditorViewModel.loaded() = withTimeout(5_000) {
        state.first { it.original != null && !it.loading }
    }

    private suspend fun SubscriptionEditorViewModel.idle() = withTimeout(5_000) { state.first { !it.saving } }

    @Test fun persistedEditRefreshesBaselineAndRevertingUrlSendsAnotherEdit() = runBlocking {
        var diskUrl = "https://example.invalid/a"
        val repo = SubscriptionRepository(catalog { args ->
            when (args.method + " " + args.path) {
                "GET /subscriptions/sample" -> editor("sample", diskUrl)
                "PUT /subscriptions/sample" -> {
                    diskUrl = args.text("url")
                    persisted("sample")
                }
                else -> error("Unexpected command")
            }
        })
        val vm = SubscriptionEditorViewModel(repo, this)
        vm.load("sample")
        vm.loaded()
        vm.update { it.copy(url = "https://example.invalid/b") }
        vm.save()
        vm.idle()
        assertEquals(diskUrl, vm.state.value.original!!.url)
        assertEquals(UiText.Plain("persisted failure"), vm.state.value.error)
        assertFalse(vm.state.value.saved)
        vm.update { it.copy(url = "https://example.invalid/a") }
        vm.save()
        vm.idle()
        assertEquals(2, calls.count { it.method == "PUT" && it.path == "/subscriptions/sample" })
        assertEquals("https://example.invalid/a", diskUrl)
    }

    @Test fun persistedAddWithFailedReadCannotAddAgainAndRetainsLaterDraft() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var diskUrl = "https://example.invalid/a"
        var readFailed = true
        val repo = SubscriptionRepository(catalog { args ->
            when (args.method + " " + args.path) {
                "POST /subscriptions" -> {
                    entered.complete(Unit)
                    withTimeout(5_000) { release.await() }
                    persisted("sample")
                }
                "GET /subscriptions/sample" -> {
                    if (readFailed) throw ModuleApiException("subscription.read_failed", "read failure")
                    editor("sample", diskUrl, "Assigned name")
                }
                "PUT /subscriptions/sample" -> {
                    diskUrl = args.text("url")
                    output("""{"group_id":"sample","persisted":true}""")
                }
                else -> error("Unexpected command")
            }
        })
        val vm = SubscriptionEditorViewModel(repo, this)
        vm.load("")
        vm.update { it.copy(url = diskUrl) }
        vm.save()
        withTimeout(5_000) { entered.await() }
        vm.update { it.copy(url = "https://example.invalid/later") }
        release.complete(Unit)
        vm.idle()
        assertEquals("sample", vm.state.value.id)
        assertTrue(vm.state.value.baselineReadRequired)
        assertEquals("https://example.invalid/later", vm.state.value.draft.url)
        assertEquals(UiText.Plain("persisted failure"), vm.state.value.error)
        vm.load("")
        assertEquals("sample", vm.state.value.id)
        vm.save()
        vm.idle()
        assertEquals(1, calls.count { it.method == "POST" && it.path == "/subscriptions" })
        readFailed = false
        vm.save()
        vm.idle()
        assertEquals(1, calls.count { it.method == "POST" && it.path == "/subscriptions" })
        assertEquals(1, calls.count { it.method == "PUT" && it.path == "/subscriptions/sample" })
        assertEquals("https://example.invalid/later", diskUrl)
        assertEquals("Assigned name", vm.state.value.draft.name)
    }

    @Test fun olderEditorLoadCannotReplaceNewIdentityOrDraft() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val repo = SubscriptionRepository(catalog { args ->
            if (args.path == "/subscriptions/old") withContext(NonCancellable) {
                entered.complete(Unit)
                withTimeout(5_000) { release.await() }
                editor("old", "https://example.invalid/old")
            } else editor("new", "https://example.invalid/new")
        })
        val vm = SubscriptionEditorViewModel(repo, this)
        vm.load("old")
        withTimeout(5_000) { entered.await() }
        val oldJob = coroutineContext[Job]!!.children.single()
        vm.load("new")
        vm.loaded()
        vm.update { it.copy(name = "Unsaved") }
        release.complete(Unit)
        withTimeout(5_000) { oldJob.join() }
        assertEquals("new", vm.state.value.id)
        assertEquals("Unsaved", vm.state.value.draft.name)
    }

    @Test fun cancellationDoesNotPublishErrorOrStartBaselineReadAfterCommittedWrite() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val scope = CoroutineScope(coroutineContext + Job())
        val repo = SubscriptionRepository(catalog { args ->
            assertEquals("POST", args.method)
            assertEquals("/subscriptions", args.path)
            entered.complete(Unit)
            withTimeout(5_000) { release.await() }
            output("""{"group_id":"sample","persisted":true}""")
        })
        val vm = SubscriptionEditorViewModel(repo, scope)
        vm.load("")
        vm.update { it.copy(url = "https://example.invalid/a") }
        vm.save()
        withTimeout(5_000) { entered.await() }
        val write = scope.coroutineContext[Job]!!.children.single()
        scope.cancel()
        release.complete(Unit)
        withTimeout(5_000) { write.join() }
        assertEquals(1, calls.size)
        assertEquals(UiText.Empty, vm.state.value.error)
        assertFalse(vm.state.value.saved)
    }

    @Test fun persistedDetailsUpdateRefreshesDataWithoutClearingError() = runBlocking {
        var name = "Before"
        val cat = catalog { args ->
            when (args.method + " " + args.path) {
                "GET /subscriptions/sample" -> output("""{"group":{"id":"sample","name":"$name","type":"subscription"}}""")
                "GET /subscriptions/sample/history" -> output("[]")
                "POST /subscriptions/sample/update" -> { name = "After"; persisted("sample") }
                else -> error("Unexpected command")
            }
        }
        val vm = SubscriptionDetailsViewModel(SubscriptionRepository(cat), NodeRepository(cat), this)
        vm.load("sample")
        withTimeout(5_000) { vm.state.first { it.details != null && !it.loading } }
        vm.update("sample")
        withTimeout(5_000) { vm.state.first { it.details?.group?.name == "After" && !it.loading } }
        assertEquals(UiText.Plain("persisted failure"), vm.state.value.error)
    }

    @Test fun mutationInvalidatesOlderSubscriptionListRead() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var reads = 0
        val repo = SubscriptionRepository(catalog { args ->
            if (args.path == "/subscriptions") {
                if (++reads == 1) withContext(NonCancellable) {
                    entered.complete(Unit)
                    withTimeout(5_000) { release.await() }
                    output("""[{"id":"sample","name":"Old","type":"subscription"}]""")
                } else output("""[{"id":"sample","name":"New","type":"subscription"}]""")
            } else output("{}")
        })
        val vm = SubscriptionsViewModel(repo, this)
        vm.setVisible(true)
        withTimeout(5_000) { entered.await() }
        val oldJobs = coroutineContext[Job]!!.children.toList()
        vm.updateSubscription("sample")
        withTimeout(5_000) { vm.state.first { it.groups.firstOrNull()?.name == "New" } }
        release.complete(Unit)
        vm.setVisible(false)
        withTimeout(5_000) { oldJobs.forEach { it.join() } }
        assertEquals("New", vm.state.value.groups.single().name)
        assertEquals(UiText.Empty, vm.state.value.error)
    }

    @Test fun persistedRemoveClearsDeletedDetailsAndKeepsErrorWithoutSuccessNavigation() = runBlocking {
        var navigated = false
        val cat = catalog { args ->
            when (args.method + " " + args.path) {
                "GET /subscriptions/sample" -> output("""{"group":{"id":"sample","name":"Sample","type":"subscription"}}""")
                "GET /subscriptions/sample/history" -> output("[]")
                "DELETE /subscriptions/sample" -> persisted("sample")
                else -> error("Unexpected command")
            }
        }
        val vm = SubscriptionDetailsViewModel(SubscriptionRepository(cat), NodeRepository(cat), this)
        vm.load("sample")
        withTimeout(5_000) { vm.state.first { it.details != null && !it.loading } }
        vm.remove("sample") { navigated = true }
        withTimeout(5_000) { vm.state.first { it.operation.isEmpty() && it.error != UiText.Empty } }
        assertNull(vm.state.value.details)
        assertTrue(vm.state.value.history.isEmpty())
        assertEquals(UiText.Plain("persisted failure"), vm.state.value.error)
        assertFalse(navigated)
    }

    @Test fun olderDetailsLoadCannotReplaceNewGroup() = runBlocking {
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val cat = catalog { args ->
            if (args.path == "/subscriptions/old") withContext(NonCancellable) {
                entered.complete(Unit)
                withTimeout(5_000) { release.await() }
                output("""{"group":{"id":"old","name":"Old","type":"local"}}""")
            } else output("""{"group":{"id":"new","name":"New","type":"local"}}""")
        }
        val vm = SubscriptionDetailsViewModel(SubscriptionRepository(cat), NodeRepository(cat), this)
        vm.load("old")
        withTimeout(5_000) { entered.await() }
        val oldJob = coroutineContext[Job]!!.children.single()
        vm.load("new")
        withTimeout(5_000) { vm.state.first { it.details?.group?.id == "new" && !it.loading } }
        release.complete(Unit)
        withTimeout(5_000) { oldJob.join() }
        assertEquals("new", vm.state.value.details!!.group.id)
        assertEquals(UiText.Empty, vm.state.value.error)
    }
}
