package com.fanjv.netproxy.feature.apps.presentation

import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.command.NetProxyCtlOutput
import com.fanjv.netproxy.core.command.NetProxyCtlTransport
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import com.fanjv.netproxy.feature.apps.data.AppPackageRepository
import com.fanjv.netproxy.feature.apps.data.AppPolicyRepository
import com.fanjv.netproxy.feature.apps.model.AppProxyConfig
import java.util.concurrent.ConcurrentLinkedDeque
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import kotlin.coroutines.CoroutineContext
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineDispatcher
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
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import org.junit.Assert.*
import org.junit.Test

class AppsViewModelTest {
    private class Transport : NetProxyCtlTransport {
        @Volatile var config = AppProxyConfig(mode = "whitelist")
        val calls = CopyOnWriteArrayList<List<String>>()
        var before: suspend (List<String>) -> Unit = {}
        var after: suspend (List<String>) -> Unit = {}

        override suspend fun execute(arguments: List<String>, timeoutMillis: Long): NetProxyCtlOutput {
            calls += arguments
            before(arguments)
            config = when (arguments[1]) {
                "mode" -> config.copy(enabled = true, mode = arguments[2])
                "enable" -> config.copy(enabled = true)
                "disable" -> config.copy(enabled = false)
                "add", "remove" -> {
                    val items = (if (config.mode == "blacklist") config.bypassApps else config.proxyApps)
                        .split(',').filter(String::isNotBlank).toSet()
                    val updated = (if (arguments[1] == "add") items + arguments[2] else items - arguments[2])
                        .joinToString(",")
                    if (config.mode == "blacklist") config.copy(bypassApps = updated)
                    else config.copy(proxyApps = updated)
                }
                else -> config
            }
            val result = config
            after(arguments)
            return NetProxyCtlOutput(true, listOf(
                """{"schema":1,"ok":true,"code":"app.test","message":"","data":${Json.encodeToString(result)}}"""
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
        queryPackages = { args ->
            when {
                args.last() == "users" -> listOf("UserInfo{0:Owner:13}")
                args.last() == "-s" -> listOf("package:system.example")
                else -> listOf("package:alpha.example", "package:beta.example")
            }
        }, resolveLabel = label,
    )

    private fun model(scope: CoroutineScope, transport: Transport, catalog: AppPackageRepository = packages(),
        dispatcher: CoroutineDispatcher? = null): AppsViewModel {
        val repository = AppPolicyRepository(NetProxyCtlClient(transport = transport))
        return if (dispatcher == null) AppsViewModel(repository, catalog, scope)
        else AppsViewModel(repository, catalog, scope, dispatcher)
    }

    private suspend fun AppsViewModel.loaded() = withTimeout(5_000) {
        state.first { !it.isLoadingApps && it.allApps.size == 2 }
    }

    private suspend fun settle() {
        val jobs = currentCoroutineContext().job.children.toList()
        withTimeout(5_000) { jobs.forEach { it.join() } }
    }

    @Test fun confirmedWriteKeepsLaterToggleAndSettingsIntentVisible() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val firstEntered = CompletableDeferred<Unit>()
        val firstRelease = CompletableDeferred<Unit>()
        val secondEntered = CompletableDeferred<Unit>()
        val secondRelease = CompletableDeferred<Unit>()
        transport.before = { args ->
            if (args[1] == "add" && args[2] == "0:alpha.example") {
                firstEntered.complete(Unit); withTimeout(10_000) { firstRelease.await() }
            } else if (args[1] == "add") {
                secondEntered.complete(Unit); withTimeout(10_000) { secondRelease.await() }
            }
        }
        vm.toggle("0:alpha.example")
        withTimeout(5_000) { firstEntered.await() }
        vm.toggle("0:beta.example")
        vm.setProxySettings(false)
        assertEquals(setOf("0:alpha.example", "0:beta.example"), vm.state.value.proxiedApps)
        firstRelease.complete(Unit)
        withTimeout(5_000) { secondEntered.await() }
        assertEquals(setOf("0:alpha.example", "0:beta.example"), vm.state.value.proxiedApps)
        assertFalse(vm.state.value.appProxyEnabled)
        assertFalse(transport.calls.any { it[1] == "disable" })
        secondRelease.complete(Unit)
        settle()
        assertFalse(vm.state.value.appProxyEnabled)
        assertEquals(listOf("list", "add", "add", "disable"), transport.calls.map { it[1] })
    }

    @Test fun rapidDoubleToggleIsAppliedInOrderWithoutOldCheckmarkReturning() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val removeEntered = CompletableDeferred<Unit>()
        val removeRelease = CompletableDeferred<Unit>()
        transport.before = { args ->
            if (args[1] == "add") { entered.complete(Unit); withTimeout(10_000) { release.await() } }
            if (args[1] == "remove") { removeEntered.complete(Unit); withTimeout(10_000) { removeRelease.await() } }
        }
        vm.toggle("0:alpha.example")
        withTimeout(5_000) { entered.await() }
        vm.toggle("0:alpha.example")
        release.complete(Unit)
        withTimeout(5_000) { removeEntered.await() }
        assertFalse(vm.state.value.proxiedApps.contains("0:alpha.example"))
        removeRelease.complete(Unit)
        settle()
        assertTrue(vm.state.value.proxiedApps.isEmpty())
        assertEquals(listOf("list", "add", "remove"), transport.calls.map { it[1] })
    }

    @Test fun failedWriteReadbackDoesNotOverwriteQueuedModeOrSelection() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val modeEntered = CompletableDeferred<Unit>()
        val modeRelease = CompletableDeferred<Unit>()
        transport.before = { args ->
            when (args[1]) {
                "disable" -> {
                    entered.complete(Unit); withTimeout(10_000) { release.await() }
                    throw NetProxyCtlException("app.update_failed", "failed write")
                }
                "mode" -> { modeEntered.complete(Unit); withTimeout(10_000) { modeRelease.await() } }
            }
        }
        vm.setProxySettings(false)
        withTimeout(5_000) { entered.await() }
        vm.setProxySettings(true, "blacklist")
        vm.toggle("0:beta.example")
        release.complete(Unit)
        withTimeout(5_000) { modeEntered.await() }
        assertEquals("blacklist", vm.state.value.appProxyMode)
        assertEquals(setOf("0:beta.example"), vm.state.value.bypassApps)
        assertTrue(vm.state.value.appProxyEnabled)
        assertEquals("failed write", vm.state.value.error)
        modeRelease.complete(Unit)
        settle()
        assertEquals("0:beta.example", transport.config.bypassApps)
        assertEquals("", vm.state.value.error)
        assertEquals(listOf("list", "disable", "list", "mode", "add"), transport.calls.map { it[1] })
    }

    @Test fun oldReadbackDuringPackageLoadingCannotReplaceNewPolicy() = runBlocking {
        val transport = Transport()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val catalog = AppPackageRepository(queryPackages = { args ->
            if (args.last() == "users") listOf("UserInfo{0:Owner:13}")
            else {
                entered.complete(Unit); withTimeout(10_000) { release.await() }
                if (args.last() == "-s") emptyList() else listOf("package:alpha.example", "package:beta.example")
            }
        }, resolveLabel = { it })
        val vm = model(this, transport, catalog)
        vm.load()
        withTimeout(5_000) { entered.await() }
        vm.setProxySettings(false)
        withTimeout(5_000) {
            while (transport.config.enabled) yield()
        }
        release.complete(Unit)
        vm.loaded()
        assertFalse(vm.state.value.appProxyEnabled)
    }

    @Test fun failedModeDoesNotApplyDependentSelectionToOppositeListOrReplayIt() = runBlocking {
        val transport = Transport().apply {
            config = AppProxyConfig(mode = "blacklist", bypassApps = "0:beta.example")
        }
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { args ->
            if (args[1] == "mode") {
                entered.complete(Unit)
                withTimeout(10_000) { release.await() }
                throw NetProxyCtlException("app.mode_invalid", "failed mode")
            }
        }
        vm.setProxySettings(true, "whitelist")
        withTimeout(5_000) { entered.await() }
        vm.toggle("0:alpha.example")
        assertEquals(setOf("0:alpha.example"), vm.state.value.proxyApps)
        release.complete(Unit)
        settle()
        assertEquals("blacklist", vm.state.value.appProxyMode)
        assertEquals("0:beta.example", transport.config.bypassApps)
        assertTrue(vm.state.value.proxyApps.isEmpty())
        assertEquals("failed mode", vm.state.value.error)
        assertEquals(listOf("list", "mode", "list"), transport.calls.map { it[1] })
        transport.before = {}
        vm.setProxySettings(true, "whitelist")
        settle()
        assertTrue(transport.config.proxyApps.isEmpty())
        vm.toggle("0:alpha.example")
        settle()
        assertEquals("0:alpha.example", transport.config.proxyApps)
        assertEquals("0:beta.example", transport.config.bypassApps)
    }

    @Test fun newestFilterWinsWhenQueuedCalculationsRunInReverseOrder() = runBlocking {
        val dispatcher = ManualDispatcher()
        val vm = model(this, Transport(), dispatcher = dispatcher)
        vm.load()
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.masterAppList.isNotEmpty() } }
        dispatcher.drain(); yield()
        vm.updateSearch("alpha")
        yield()
        vm.setReverseSort(true)
        vm.updateSearch("beta")
        yield()
        dispatcher.newest(); yield()
        dispatcher.drain(); yield()
        assertEquals("beta", vm.state.value.appSearchQuery)
        assertEquals(listOf("beta.example"), vm.state.value.searchResults.map { it.packageName })
        assertEquals(listOf("beta.example", "alpha.example"), vm.state.value.allApps.map { it.packageName })
        vm.updateSearch("")
        yield(); dispatcher.drain(); yield()
        assertTrue(vm.state.value.searchResults.isEmpty())
    }

    @Test fun failedModeAndReadbackCannotApplySelectionUsingUnconfirmedMode() = runBlocking {
        val transport = Transport()
        val vm = model(this, transport)
        vm.load(); vm.loaded()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        transport.before = { args ->
            if (args[1] == "mode") {
                entered.complete(Unit)
                withTimeout(10_000) { release.await() }
                transport.config = AppProxyConfig(mode = "blacklist")
                throw NetProxyCtlException("app.mode_invalid", "failed mode")
            }
            if (args[1] == "list") throw NetProxyCtlException("app.read_failed", "failed readback")
        }
        vm.setProxySettings(true, "whitelist")
        withTimeout(5_000) { entered.await() }
        vm.toggle("0:alpha.example")
        release.complete(Unit)
        settle()
        assertFalse(transport.calls.any { it[1] == "add" || it[1] == "remove" })
        assertEquals("", transport.config.bypassApps)
        assertEquals("failed mode", vm.state.value.error)
    }

    @Test fun cancellationStopsCpuFilteringBeforeTraversingRemainingApps() = runBlocking {
        val snapshot = AppsUiState(masterAppList = List(100) {
            AppInfoModel("app$it.example", "App $it", false)
        })
        var reads = 0
        var returned = false
        val calculation = launch {
            val context = currentCoroutineContext()
            val labels = object : AbstractMap<String, String>() {
                override val entries = emptySet<Map.Entry<String, String>>()
                override fun get(key: String): String {
                    reads++
                    context.job.cancel()
                    return key
                }
            }
            calculateAppsList(snapshot, labels)
            returned = true
        }
        calculation.join()
        assertTrue(calculation.isCancelled)
        assertFalse(returned)
        assertEquals(1, reads)
    }

    @Test fun forceRefreshReloadsListingLabelsAndIconGeneration() = runBlocking {
        val label = AtomicReference("Before")
        val vm = model(this, Transport(), packages { label.get() })
        vm.load(); vm.loaded()
        assertTrue(vm.state.value.allApps.all { it.label == "Before" })
        val revision = AppIconCache.revision
        label.set("After")
        vm.load(force = true)
        withTimeout(5_000) { vm.state.first { !it.isLoadingApps && it.allApps.all { app -> app.label == "After" } } }
        assertTrue(AppIconCache.revision > revision)
    }

    @Test fun forcedLoadSupersedesInFlightListingWithoutCancellationError() = runBlocking {
        val calls = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val catalog = AppPackageRepository(queryPackages = { args ->
            when {
                args.last() == "users" && calls.incrementAndGet() == 1 -> {
                    entered.complete(Unit)
                    withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                    listOf("UserInfo{10:Old:13}")
                }
                args.last() == "users" -> listOf("UserInfo{0:Owner:13}")
                args.last() == "-s" -> emptyList()
                else -> listOf("package:alpha.example", "package:beta.example")
            }
        }, resolveLabel = { it })
        val vm = model(this, Transport(), catalog)
        vm.load()
        withTimeout(5_000) { entered.await() }
        vm.load(force = true)
        vm.loaded()
        release.complete(Unit)
        settle()
        assertTrue(vm.state.value.masterAppList.all { it.userId == "0" })
        assertEquals("", vm.state.value.error)
    }

    @Test fun cancelledWriteDoesNotPublishFailureOrApplyLateResult() = runBlocking {
        val transport = Transport()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        lateinit var vm: AppsViewModel
        coroutineScope {
            val worker = launch {
                vm = model(this, transport)
                vm.load(); vm.loaded()
                transport.after = {
                    entered.complete(Unit)
                    withContext(NonCancellable) { withTimeout(10_000) { release.await() } }
                }
                vm.setProxySettings(false)
            }
            withTimeout(5_000) { entered.await() }
            worker.cancel(CancellationException("page closed"))
            release.complete(Unit)
            worker.join()
        }
        assertEquals("", vm.state.value.error)
        assertFalse(vm.state.value.appProxyEnabled)
    }
}
