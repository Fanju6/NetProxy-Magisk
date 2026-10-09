package com.fanjv.netproxy.feature.apps.presentation

import androidx.compose.runtime.saveable.SaverScope
import com.fanjv.netproxy.core.command.*
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

    private class Transport : NetProxyCtlTransport {
        @Volatile var config = AppProxyConfig(mode = "whitelist")
        var revision = 0
        val calls = CopyOnWriteArrayList<List<String>>()
        var before: suspend (List<String>) -> Unit = {}
        var after: suspend (List<String>) -> Unit = {}
        private val json = Json { encodeDefaults = true }
        val writes get() = calls.count { it[1] == "apply" }

        override suspend fun execute(arguments: List<String>, timeoutMillis: Long): NetProxyCtlOutput {
            calls += arguments
            before(arguments)
            val data = if (arguments[1] == "apply") {
                if (arguments[3] != revision.toString()) throw NetProxyCtlException("config.conflict", "conflict")
                config = Json.decodeFromJsonElement(AppProxyConfig.serializer(),
                    Json.parseToJsonElement(File(arguments.last()).readText()).jsonObject.getValue("app"))
                revision++
                """{"revision":"$revision"}"""
            } else {
                val content = json.encodeToString(mapOf("app" to config))
                """{"revision":"$revision","content":${Json.encodeToString(content)}}"""
            }
            after(arguments)
            return NetProxyCtlOutput(true, listOf(
                """{"schema":1,"ok":true,"code":"config.test","message":"","data":$data}"""
            ), emptyList())
        }
    }

    private class ManualDispatcher : CoroutineDispatcher() {
        val tasks = ConcurrentLinkedDeque<Runnable>()
        override fun dispatch(context: CoroutineContext, block: Runnable) { tasks.add(block) }
        fun newest() { tasks.pollLast()?.run() }
        fun drain() { while (tasks.isNotEmpty()) tasks.pollFirst()?.run() }
    }

    private fun packages(label: (String) -> String = { it }) = AppPackageRepository(
        queryPackages = { args -> when {
            args.last() == "users" -> listOf("UserInfo{0:Owner:13}")
            args.last() == "-s" -> listOf("package:system.example")
            else -> listOf("package:alpha.example", "package:beta.example")
        } }, resolveLabel = label,
    )

    private fun model(scope: CoroutineScope, transport: Transport, catalog: AppPackageRepository = packages(),
        dispatcher: CoroutineDispatcher = Dispatchers.Default, saveScope: CoroutineScope = scope,
        onSaveFailure: (String) -> Unit = {}) = AppsViewModel(
        AppPolicyRepository(ConfigRepository(NetProxyCtlClient(transport = transport), CommandFileStore(folder.root))),
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

    @Test fun leavingAndClearingPageDoesNotCancelPolicyCommit() = runBlocking {
        val pageScope = CoroutineScope(coroutineContext + Job())
        val transport = Transport()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { if (it[1] == "apply") {
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
            transport.before = { if (it[1] == "apply") throw NetProxyCtlException("config.conflict", "conflict") }
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
        transport.before = { if (it[1] == "apply" && transport.writes == 1) {
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
        transport.before = { if (it[1] == "apply" && transport.writes == 1) {
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
        transport.before = { if (it[1] == "apply" && transport.writes == 1) {
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
        transport.before = { if (it[1] == "apply" && transport.writes == 1) {
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
        transport.before = { if (it[1] == "apply") throw NetProxyCtlException("config.conflict", "conflict") }
        assertFalse(vm.flushPolicy())
        assertEquals("blacklist", vm.state.value.appProxyMode)
        assertEquals(setOf("0:beta.example"), vm.state.value.proxiedApps)
        assertTrue(vm.state.value.requiresPolicyReload)
        vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps } }
        assertFalse(vm.flushPolicy())
        assertEquals(1, transport.writes)
        assertEquals(1, transport.calls.count { it[1] == "read" })
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
        transport.after = { if (it[1] == "apply") throw NetProxyCtlException("subscription.runtime_sync_failed", "failed") }
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
        transport.before = { if (it[1] == "apply") {
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
        transport.before = { throw NetProxyCtlException("config.read_failed", "failed") }
        vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps } }
        dispatcher.drain(); yield()
        assertFalse(vm.state.value.isFilteringApps)
        assertEquals("failed", vm.state.value.error)
    }

    @Test fun cancellationStopsCpuFilteringBeforeTraversingRemainingApps() = runBlocking {
        val lookups = AtomicInteger()
        val entries = (0..1_000).map { AppInfoModel("pkg.$it", "App $it") }
        val job = launch {
            val current = currentCoroutineContext().job
            val labels = object : Map<String, String> by emptyMap() {
                override fun get(key: String): String? {
                    if (lookups.incrementAndGet() == 10) current.cancel()
                    return null
                }
            }
            calculateAppsList(AppsUiState(masterAppList = entries), labels)
        }
        job.join()
        assertTrue(job.isCancelled)
        assertEquals(10, lookups.get())
    }

    @Test fun forceRefreshReloadsListingLabelsAndIconGeneration() = runBlocking {
        val label = AtomicReference("Before")
        val vm = model(this, Transport(), packages { label.get() })
        vm.load(); vm.loaded()
        val revision = AppIconCache.revision
        label.set("After"); vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.allApps.all { app -> app.label == "After" } } }
        assertTrue(AppIconCache.revision > revision)
    }
}
