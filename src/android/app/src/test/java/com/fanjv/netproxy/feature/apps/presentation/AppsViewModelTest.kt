package com.fanjv.netproxy.feature.apps.presentation

import com.fanjv.netproxy.core.api.*

import androidx.compose.runtime.saveable.SaverScope
import com.fanjv.netproxy.core.config.*
import com.fanjv.netproxy.core.ui.component.SearchStatus
import com.fanjv.netproxy.feature.apps.data.*
import com.fanjv.netproxy.feature.apps.model.AppProxyConfig
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import java.io.File
import java.util.concurrent.ConcurrentLinkedDeque
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import kotlin.coroutines.CoroutineContext
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.*
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

@OptIn(ExperimentalCoroutinesApi::class)
class AppsViewModelTest {
    @get:Rule val folder = TemporaryFolder()

    private class Transport : TestApiTransport() {
        @Volatile var config = AppProxyConfig(enabled = true, mode = "whitelist")
        var revision = 0
        val calls = CopyOnWriteArrayList<ApiTestRequest>()
        var before: suspend (ApiTestRequest) -> Unit = {}
        var after: suspend (ApiTestRequest) -> Unit = {}
        private val json = Json { encodeDefaults = true }
        val writes get() = calls.count { it.method == "PUT" }

        override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
            calls += request
            before(request)
            val data = if (request.method == "PUT") {
                if (request.text("revision") != revision.toString()) throw ModuleApiException("config.conflict", "conflict")
                config = Json.decodeFromJsonElement(AppProxyConfig.serializer(),
                    Json.parseToJsonElement(request.text("content")).jsonObject.getValue("app"))
                revision++
                """{"revision":"$revision"}"""
            } else {
                val content = json.encodeToString(mapOf("app" to config))
                """{"revision":"$revision","content":${Json.encodeToString(content)}}"""
            }
            after(request)
            return apiSuccess(data)
        }
    }

    private class ManualDispatcher : CoroutineDispatcher() {
        val tasks = ConcurrentLinkedDeque<Runnable>()
        override fun dispatch(context: CoroutineContext, block: Runnable) { tasks.add(block) }
        fun newest() { tasks.pollLast()?.run() }
        fun drain() { while (tasks.isNotEmpty()) tasks.pollFirst()?.run() }
    }

    private fun packages(label: (String) -> String = { it }) = AppPackageRepository(
        ModuleApiClient(TestApiTransport { request ->
            val data = when (request.path) {
                "/android/apps" -> {
                    assertTrue(request.query.isEmpty())
                    val apps = listOf("alpha.example", "beta.example", "system.example").map { name ->
                        AndroidApp(name, label(name), 10000, name.startsWith("system"), 1, 1, "")
                    }
                    """{"users":[{"id":0,"name":"Owner","revision":"apps","apps":${Json.encodeToString(apps)}}]}"""
                }
                else -> error("Unexpected request")
            }
            apiSuccess(data)
        })
    )

    private fun model(scope: CoroutineScope, transport: Transport, catalog: AppPackageRepository = packages(),
        dispatcher: CoroutineDispatcher = Dispatchers.Default, saveScope: CoroutineScope = scope,
        onSaveFailure: (String) -> Unit = {}) = AppsViewModel(
        AppPolicyRepository(ConfigRepository(ModuleApiClient(transport = transport))),
        catalog, scope, dispatcher, ConfigurationWrites(saveScope) { onSaveFailure(it.message.orEmpty()) },
    )

    private suspend fun AppsViewModel.loaded() = withTimeout(5_000) {
        state.first { !it.isLoadingApps && it.allApps.size == 2 }
    }

    private suspend fun AppsViewModel.flushPolicy(): Boolean = withTimeout(5_000) {
        do {
            requestPolicyFlush()
            state.first { !it.isSavingPolicy }
        } while (state.value.hasPendingPolicy && !state.value.requiresPolicyReload)
        !state.value.requiresPolicyReload && !state.value.hasPendingPolicy
    }

    @Test fun oneSnapshotLoadsAllUsersInParallelWithPolicy() = runBlocking {
        val transport = Transport()
        val policyEntered = CompletableDeferred<Unit>()
        val snapshotEntered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val calls = AtomicInteger()
        transport.before = {
            policyEntered.complete(Unit)
            withTimeout(5_000) { release.await() }
        }
        val catalog = AppPackageRepository(ModuleApiClient(TestApiTransport { request ->
            assertEquals("/android/apps", request.path)
            assertTrue(request.query.isEmpty())
            calls.incrementAndGet()
            snapshotEntered.complete(Unit)
            withTimeout(5_000) { release.await() }
            val owner = AndroidApp("same.example", "Owner app", 10001, false, 1, 1, "a".repeat(64))
            val work = owner.copy(label = "Work app", uid = 1010001, iconKey = "b".repeat(64))
            apiSuccess(Json.encodeToString(AndroidAppsSnapshot(listOf(
                AndroidUserApps(0, "Owner", "owner", listOf(owner)),
                AndroidUserApps(10, "Work", "work", listOf(work)),
            ))))
        }))
        val vm = model(this, transport, catalog)
        try {
            vm.load()
            withTimeout(5_000) { policyEntered.await(); snapshotEntered.await() }
        } finally { release.complete(Unit) }
        vm.loaded()
        assertEquals(listOf("0:same.example", "10:same.example"), vm.state.value.masterAppList.map { it.id })
        assertEquals(listOf("Owner app", "Work app"), vm.state.value.masterAppList.map { it.label })
        assertEquals(listOf("a".repeat(64), "b".repeat(64)), vm.state.value.masterAppList.map { it.iconKey })
        repeat(3) { vm.load() }
        assertEquals(1, calls.get())
    }

    @Test fun applicationsWithoutIconsRemainSelectableWithoutIconRequests() = runBlocking {
        val iconRequests = AtomicInteger()
        val catalog = AppPackageRepository(ModuleApiClient(object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
                val apps = listOf("alpha.example", "beta.example").map {
                    AndroidApp(it, it, 10001, false, 1, 1, "")
                }
                return apiSuccess(Json.encodeToString(AndroidAppsSnapshot(listOf(AndroidUserApps(0, "Owner", "apps", apps)))))
            }
            override suspend fun image(path: String): ByteArray {
                iconRequests.incrementAndGet()
                return byteArrayOf()
            }
        }))
        val vm = model(this, Transport(), catalog)
        vm.load(); vm.loaded()
        assertTrue(vm.state.value.masterAppList.all { it.iconKey.isEmpty() })
        val revision = AppIconCache.revision
        vm.loadIcons(vm.state.value.masterAppList.map { it.id })
        assertEquals(0, iconRequests.get())
        assertEquals(revision, AppIconCache.revision)
        vm.toggle("0:alpha.example")
        assertEquals(setOf("0:alpha.example"), vm.state.value.proxyApps)
        assertEquals(2, vm.state.value.masterAppList.size)
    }

    @Test fun snapshotFailureKeepsLoadedApplicationsAndIconKeys() = runBlocking {
        var failing = false
        val catalog = AppPackageRepository(ModuleApiClient(TestApiTransport {
            if (failing) throw ModuleApiException("android.query_failed", "fixture")
            val apps = listOf("alpha.example", "beta.example").map {
                AndroidApp(it, it, 10001, false, 1, 1, "a".repeat(64))
            }
            apiSuccess(Json.encodeToString(AndroidAppsSnapshot(listOf(AndroidUserApps(0, "Owner", "owner", apps)))))
        }))
        val vm = model(this, Transport(), catalog)
        vm.load(); vm.loaded()
        val apps = vm.state.value.masterAppList
        val revision = AppIconCache.revision
        failing = true
        vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.error.isNotBlank() } }
        assertSame(apps, vm.state.value.masterAppList)
        assertEquals(revision, AppIconCache.revision)
        assertEquals("fixture", vm.state.value.error)
    }

    @Test fun defaultPolicyIsOffAndLoadingPreservesSavedPolicy() = runBlocking {
        assertFalse(AppProxyConfig().enabled)
        assertFalse(AppsUiState().appProxyEnabled)
        for (enabled in listOf(false, true)) {
            val scope = CoroutineScope(coroutineContext + Job())
            val transport = Transport().apply {
                config = AppProxyConfig(enabled = enabled, mode = "whitelist",
                    proxyApps = listOf("0:alpha.example"))
            }
            try {
                val vm = model(scope, transport)
                vm.load(); vm.loaded()
                assertEquals(enabled, vm.state.value.appProxyEnabled)
                assertEquals("whitelist", vm.state.value.appProxyMode)
                assertEquals(setOf("0:alpha.example"), vm.state.value.proxyApps)
                assertEquals(0, transport.writes)
            } finally { scope.cancel() }
        }
    }

    @Test fun leavingAndClearingPageDoesNotCancelPolicyCommit() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val transport = Transport()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { if (it.method == "PUT") {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        val vm = model(pageScope, transport, saveScope = this)
        try {
            vm.load(); vm.loaded()
            vm.toggle("0:alpha.example")
            vm.requestPolicyFlush()
            pageScope.cancel()
            withTimeout(5_000) { entered.await() }
            assertEquals(emptyList<String>(), transport.config.proxyApps)
            release.complete(Unit)
            withTimeout(5_000) { vm.state.first { !it.isSavingPolicy && !it.hasPendingPolicy } }
            assertEquals(listOf("0:alpha.example"), transport.config.proxyApps)
            assertEquals(1, transport.writes)
        } finally { release.complete(Unit); pageScope.cancel() }
    }

    @Test fun failedCommitAfterLeavingReportsErrorWithoutRetry() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val transport = Transport()
        val errors = mutableListOf<String>()
        val vm = model(pageScope, transport, saveScope = this, onSaveFailure = errors::add)
        try {
            vm.load(); vm.loaded()
            transport.before = { if (it.method == "PUT") throw ModuleApiException("config.conflict", "conflict") }
            vm.toggle("0:alpha.example")
            vm.requestPolicyFlush(); pageScope.cancel()
            withTimeout(5_000) { vm.state.first { it.requiresPolicyReload && !it.isSavingPolicy } }
            assertEquals(listOf("conflict"), errors)
            assertTrue(vm.state.value.hasPendingPolicy)
            vm.requestPolicyFlush()
            assertEquals(1, transport.writes)
        } finally { pageScope.cancel() }
    }

    @Test fun editsHaveNoIdleTimerAndLeaveMergesAllConfirmedChanges() = runTest {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load()
        withContext(Dispatchers.Default) { vm.loaded() }
        vm.toggle("0:alpha.example")
        vm.setProxySettings(true, "blacklist")
        vm.toggle("0:beta.example")
        vm.setProxySettings(false)
        advanceTimeBy(60_000)
        runCurrent()
        assertEquals(0, transport.writes)
        assertTrue(vm.state.value.hasPendingPolicy)
        assertTrue(withContext(Dispatchers.Default) { vm.flushPolicy() })
        assertEquals(1, transport.writes)
        assertEquals(listOf("0:alpha.example"), transport.config.proxyApps)
        assertEquals(listOf("0:beta.example"), transport.config.bypassApps)
        assertFalse(transport.config.enabled)
    }

    @Test fun returningToOriginalPolicyDoesNotWrite() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        vm.toggle("0:alpha.example"); vm.toggle("0:alpha.example")
        vm.setProxySettings(false); vm.setProxySettings(true)
        assertFalse(vm.state.value.hasPendingPolicy)
        assertTrue(vm.flushPolicy())
        assertEquals(0, transport.writes)
    }

    @Test fun queuedSaveOnlyIncludesSelectionAtItsTrigger() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        vm.toggle("0:alpha.example"); vm.requestPolicyFlush()
        vm.toggle("0:beta.example")
        withTimeout(5_000) { vm.state.first { !it.isSavingPolicy } }
        assertEquals(listOf("0:alpha.example"), transport.config.proxyApps)
        assertEquals(setOf("0:alpha.example", "0:beta.example"), vm.state.value.proxyApps)
        assertTrue(vm.state.value.hasPendingPolicy)
        assertTrue(vm.flushPolicy())
        assertEquals(setOf("0:alpha.example", "0:beta.example"), transport.config.proxyApps.toSet())
        assertEquals(2, transport.writes)
    }

    @Test fun repeatedTriggerDuringSaveDoesNotSubmitLaterForegroundEdits() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { if (it.method == "PUT" && transport.writes == 1) {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        vm.toggle("0:alpha.example"); vm.requestPolicyFlush()
        withTimeout(5_000) { entered.await() }
        vm.toggle("0:beta.example"); vm.requestPolicyFlush()
        vm.toggle("0:alpha.example")
        release.complete(Unit)
        withTimeout(5_000) { vm.state.first { !it.isSavingPolicy } }
        assertEquals(setOf("0:alpha.example", "0:beta.example"), transport.config.proxyApps.toSet())
        assertEquals(setOf("0:beta.example"), vm.state.value.proxyApps)
        assertTrue(vm.state.value.hasPendingPolicy)
        assertTrue(vm.flushPolicy())
        assertEquals(listOf("0:beta.example"), transport.config.proxyApps)
        assertEquals(3, transport.writes)
    }

    @Test fun backgroundSaveKeepsNewIntentAndRepeatedFlushDoesNotDuplicateWrites() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { if (it.method == "PUT" && transport.writes == 1) {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        vm.toggle("0:alpha.example")
        vm.requestPolicyFlush()
        withTimeout(5_000) { entered.await() }
        vm.requestPolicyFlush()
        vm.toggle("0:beta.example")
        vm.setProxySettings(false)
        release.complete(Unit)
        assertTrue(vm.flushPolicy())
        assertEquals(setOf("0:alpha.example", "0:beta.example"), vm.state.value.proxiedApps)
        assertEquals(2, transport.writes)
        assertFalse(transport.config.enabled)
        vm.toggle("0:alpha.example")
        yield()
        assertEquals(2, transport.writes)
        assertTrue(vm.state.value.hasPendingPolicy)
    }

    @Test fun revertingDuringWriteDoesNotRestoreAnOldCheckmarkOrSkipPendingRemoval() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { if (it.method == "PUT" && transport.writes == 1) {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        vm.toggle("0:alpha.example"); vm.requestPolicyFlush()
        withTimeout(5_000) { entered.await() }
        vm.toggle("0:alpha.example")
        assertTrue(vm.state.value.hasPendingPolicy)
        assertTrue(vm.state.value.proxiedApps.isEmpty())
        release.complete(Unit)
        assertTrue(vm.flushPolicy())
        assertTrue(transport.config.proxyApps.isEmpty())
        assertEquals(2, transport.writes)
    }

    @Test fun backgroundSaveLeavesLaterForegroundEditsForNextLeave() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { if (it.method == "PUT" && transport.writes == 1) {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        vm.toggle("0:alpha.example"); vm.requestPolicyFlush()
        withTimeout(5_000) { entered.await() }
        vm.toggle("0:beta.example")
        release.complete(Unit)
        withTimeout(5_000) { vm.state.first { !it.isSavingPolicy } }
        assertEquals(1, transport.writes)
        assertEquals(listOf("0:alpha.example"), transport.config.proxyApps)
        assertTrue(vm.state.value.hasPendingPolicy)
        assertTrue(vm.flushPolicy())
        assertEquals(2, transport.writes)
    }

    @Test fun failedWriteRetainsModeAndSelectionWithoutRebasingOrRetrying() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        vm.setProxySettings(true, "blacklist"); vm.toggle("0:beta.example")
        transport.before = { if (it.method == "PUT") throw ModuleApiException("config.conflict", "conflict") }
        assertFalse(vm.flushPolicy())
        assertEquals("blacklist", vm.state.value.appProxyMode)
        assertEquals(setOf("0:beta.example"), vm.state.value.proxiedApps)
        assertTrue(vm.state.value.requiresPolicyReload)
        vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps } }
        assertFalse(vm.flushPolicy())
        assertEquals(1, transport.writes)
        assertEquals(1, transport.calls.count { it.method == "GET" })
        transport.before = {}
        vm.discardPolicyAndReload()
        vm.loaded()
        assertFalse(vm.state.value.hasPendingPolicy)
        assertEquals("whitelist", vm.state.value.appProxyMode)
        vm.toggle("0:alpha.example")
        assertTrue(vm.flushPolicy())
    }

    @Test fun persistedFailureIsNotReportedAsSuccessOrAutomaticallyReplayed() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        transport.after = { if (it.method == "PUT") throw ModuleApiException("subscription.runtime_sync_failed", "failed") }
        vm.toggle("0:alpha.example")
        assertFalse(vm.flushPolicy())
        assertEquals(listOf("0:alpha.example"), transport.config.proxyApps)
        assertTrue(vm.state.value.hasPendingPolicy)
        assertFalse(vm.flushPolicy())
        assertEquals(1, transport.writes)
        transport.after = {}
        vm.discardPolicyAndReload(); vm.loaded()
        assertEquals(setOf("0:alpha.example"), vm.state.value.proxiedApps)
        assertFalse(vm.state.value.hasPendingPolicy)
    }

    @Test fun cancellingLeaveWaitDoesNotCancelStartedWrite() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { if (it.method == "PUT") {
            entered.complete(Unit); withTimeout(5_000) { release.await() }
        } }
        vm.toggle("0:alpha.example")
        val leaving = launch { vm.flushPolicy() }
        withTimeout(5_000) { entered.await() }
        leaving.cancelAndJoin()
        assertTrue(vm.state.value.isSavingPolicy)
        release.complete(Unit)
        assertTrue(vm.flushPolicy())
        assertEquals(1, transport.writes)
    }

    @Test fun searchCheckmarksNeverRebuildResultsOrScheduleFiltering() = runBlocking {
        val dispatcher = ManualDispatcher()
        val transport = Transport()
        val vm = model(this, transport, dispatcher = dispatcher)
        vm.load()
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.masterAppList.isNotEmpty() } }
        dispatcher.drain(); yield()
        vm.updateSearch(".example")
        yield(); dispatcher.drain(); yield()
        val results = vm.state.value.searchResults
        vm.toggle("0:beta.example")
        assertEquals(listOf("beta.example", "alpha.example"),
            vm.state.value.orderedApps(results).map { it.packageName })
        vm.toggle("0:alpha.example")
        assertEquals(listOf("alpha.example", "beta.example"),
            vm.state.value.orderedApps(results).map { it.packageName })
        assertSame(results, vm.state.value.searchResults)
        assertFalse(vm.state.value.isFilteringApps)
        assertTrue(dispatcher.tasks.isEmpty())
        vm.setSelectedFirst(false)
        assertSame(results, vm.state.value.searchResults)
        assertSame(results, vm.state.value.orderedApps(results))
        assertFalse(vm.state.value.isFilteringApps)
        assertTrue(dispatcher.tasks.isEmpty())
        for (enabled in listOf(false, true)) {
            vm.setProxySettings(enabled)
            assertEquals(enabled, vm.state.value.appProxyEnabled)
            assertEquals(".example", vm.state.value.appSearchQuery)
            assertSame(results, vm.state.value.searchResults)
            assertTrue(dispatcher.tasks.isEmpty())
        }
        assertEquals(0, transport.writes)
        assertEquals(1, transport.calls.size)
        assertEquals(".example", vm.state.value.appSearchQuery)
        assertTrue(vm.flushPolicy())
        assertEquals(1, transport.writes)
    }

    @Test fun selectionDuringSearchCalculationUsesLatestCheckmarks() = runBlocking {
        val dispatcher = ManualDispatcher()
        val vm = model(this, Transport(), dispatcher = dispatcher)
        vm.load()
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.masterAppList.isNotEmpty() } }
        dispatcher.drain(); yield()
        vm.updateSearch("alpha"); yield()
        vm.toggle("0:alpha.example")
        dispatcher.drain(); yield()
        assertEquals(listOf("alpha.example"), vm.state.value.searchResults.map { it.packageName })
        assertEquals(setOf("0:alpha.example"), vm.state.value.proxiedApps)
    }

    @Test fun dirtyRefreshPreservesOriginalRevisionAndDoesNotOverwriteRemoteSelection() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        vm.toggle("0:alpha.example")
        transport.config = transport.config.copy(proxyApps = listOf("0:beta.example"))
        transport.revision++
        vm.load(force = true); vm.loaded()
        assertEquals(setOf("0:alpha.example"), vm.state.value.proxiedApps)
        assertFalse(vm.flushPolicy())
        assertEquals(listOf("0:beta.example"), transport.config.proxyApps)
    }

    @Test fun selectedFirstUsesSameOrderingForNormalAndSearchLists() {
        val alpha = AppInfoModel("alpha.example", "Alpha")
        val beta = AppInfoModel("beta.example", "Beta")
        val state = AppsUiState(allApps = listOf(alpha, beta), searchResults = listOf(alpha, beta),
            proxiedApps = setOf(beta.id))
        assertEquals(listOf(beta, alpha), state.orderedApps())
        assertEquals(state.orderedApps(), state.orderedApps(state.searchResults))
        assertEquals(listOf(alpha, beta), state.searchResults)
        assertSame(state.allApps, state.copy(appSelectedFirst = false).orderedApps())
        assertSame(state.searchResults, state.copy(appSelectedFirst = false).orderedApps(state.searchResults))
        val reversed = state.copy(allApps = listOf(beta, alpha), searchResults = listOf(beta, alpha), appReverseSort = true)
        assertEquals(listOf(alpha, beta), reversed.orderedApps())
        assertEquals(reversed.orderedApps(), reversed.orderedApps(reversed.searchResults))
    }

    @Test fun searchOrderingPreservesFilterUserIdentityAndRelativeOrder() {
        val alpha = AppInfoModel("alpha.example", "Alpha")
        val owner = AppInfoModel("google.example", "Google", userId = "0")
        val work = owner.copy(userId = "10")
        val play = AppInfoModel("google.play", "Google Play")
        val search = listOf(owner, work, play)
        val state = AppsUiState(allApps = listOf(alpha) + search, searchResults = search,
            proxiedApps = setOf(alpha.id, work.id, play.id))
        assertEquals(listOf(work, play, owner), state.orderedApps(search))
        assertEquals(listOf(owner, work, play), state.copy(proxiedApps = emptySet()).orderedApps(search))
        assertEquals(listOf(owner, work, play), state.copy(proxiedApps = search.map { it.id }.toSet()).orderedApps(search))
        assertEquals(listOf(owner, work, play), state.copy(appSelectedFirst = false).orderedApps(search))
        assertEquals(listOf(owner, work, play), search)
        assertTrue(state.orderedApps(emptyList()).isEmpty())
    }

    @Test fun continuousSearchSelectionAndDeselectionUsesLatestPolicyWithoutRefiltering() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        vm.updateSearch(".example")
        withTimeout(5_000) { vm.state.first { !it.isFilteringApps && it.searchResults.size == 2 } }
        val results = vm.state.value.searchResults
        vm.toggle("0:beta.example")
        assertEquals(listOf("beta.example", "alpha.example"), vm.state.value.orderedApps(results).map { it.packageName })
        vm.toggle("0:alpha.example")
        vm.toggle("0:alpha.example")
        assertEquals(listOf("beta.example", "alpha.example"), vm.state.value.orderedApps(results).map { it.packageName })
        vm.toggle("0:beta.example")
        assertEquals(listOf("alpha.example", "beta.example"), vm.state.value.orderedApps(results).map { it.packageName })
        assertSame(results, vm.state.value.searchResults)
        assertFalse(vm.state.value.isFilteringApps)
        assertEquals(0, transport.writes)
        assertFalse(vm.state.value.hasPendingPolicy)
    }

    @Test fun searchStateRestoresExpandedQueryButNotClosingAnimation() {
        val search = SearchStatus("Search").apply { searchText = "example"; current = SearchStatus.Status.EXPANDING }
        val scope = SaverScope { true }
        val restored = with(SearchStatus.Saver) { restore(scope.save(search)!!)!! }
        assertEquals("example", restored.searchText)
        assertEquals(SearchStatus.Status.EXPANDED, restored.current)
        search.current = SearchStatus.Status.COLLAPSING
        val closed = with(SearchStatus.Saver) { restore(scope.save(search)!!)!! }
        assertEquals("", closed.searchText)
        assertTrue(closed.isCollapsed())
    }

    @Test fun searchAnimationOnlyShowsResultsAfterExpansionAndRestoresContentOnCollapse() {
        val search = SearchStatus("").apply {
            searchText = "example"
            current = SearchStatus.Status.EXPANDING
        }
        assertTrue(search.shouldExpand())
        assertFalse(search.isExpand())
        assertFalse(search.shouldCollapsed())
        search.onAnimationComplete()
        assertTrue(search.isExpand())
        assertEquals("example", search.searchText)

        search.current = SearchStatus.Status.COLLAPSING
        assertTrue(search.shouldCollapsed())
        assertFalse(search.shouldExpand())
        search.onAnimationComplete()
        assertTrue(search.isCollapsed())
        assertEquals("", search.searchText)
    }

    @Test fun interruptedExpansionCompletesCurrentCollapseInsteadOfReopeningSearch() {
        val search = SearchStatus("").apply { current = SearchStatus.Status.EXPANDING }
        search.current = SearchStatus.Status.COLLAPSING
        search.onAnimationComplete()
        assertTrue(search.isCollapsed())
        search.onAnimationComplete()
        assertTrue(search.isCollapsed())
    }

    @Test fun newestFilterWinsWhenQueuedCalculationsRunInReverseOrder() = runBlocking {
        val dispatcher = ManualDispatcher()
        val vm = model(this, Transport(), dispatcher = dispatcher)
        vm.load()
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.masterAppList.isNotEmpty() } }
        dispatcher.drain(); yield()
        vm.updateSearch("alpha"); yield()
        vm.setReverseSort(true); vm.updateSearch("beta"); yield()
        dispatcher.newest(); yield(); dispatcher.drain(); yield()
        assertEquals(listOf("beta.example"), vm.state.value.searchResults.map { it.packageName })
        assertEquals(listOf("beta.example", "alpha.example"), vm.state.value.allApps.map { it.packageName })
    }

    @Test fun failedRefreshClearsCancelledSearchCalculation() = runBlocking {
        val dispatcher = ManualDispatcher()
        val transport = Transport()
        val vm = model(this, transport, dispatcher = dispatcher)
        vm.load()
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.masterAppList.isNotEmpty() } }
        dispatcher.drain(); yield()
        vm.updateSearch("alpha"); yield()
        transport.before = { throw ModuleApiException("config.read_failed", "failed") }
        vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps } }
        dispatcher.drain(); yield()
        assertFalse(vm.state.value.isFilteringApps)
        assertEquals("failed", vm.state.value.error)
    }

    @Test fun cancellationStopsCpuFilteringBeforeTraversingRemainingApps() = runBlocking {
        val lookups = AtomicInteger()
        val job = launch {
            val current = currentCoroutineContext().job
            val entries = object : AbstractList<AppInfoModel>() {
                override val size = 1_001
                override fun get(index: Int): AppInfoModel {
                    if (lookups.incrementAndGet() == 10) current.cancel()
                    return AppInfoModel("pkg.$index", "App $index")
                }
            }
            calculateAppsList(AppsUiState(masterAppList = entries))
        }
        job.join()
        assertTrue(job.isCancelled)
        assertEquals(10, lookups.get())
    }

    @Test fun forceRefreshReloadsListingLabelsWithoutClearingIcons() = runBlocking {
        val label = AtomicReference("Before")
        val vm = model(this, Transport(), packages { label.get() })
        vm.load(); vm.loaded()
        val revision = AppIconCache.revision
        label.set("After"); vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.allApps.all { app -> app.label == "After" } } }
        assertEquals(revision, AppIconCache.revision)
    }
}
