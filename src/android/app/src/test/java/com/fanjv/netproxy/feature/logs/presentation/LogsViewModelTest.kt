package com.fanjv.netproxy.feature.logs.presentation

import com.fanjv.netproxy.core.command.*
import com.fanjv.netproxy.feature.logs.data.*
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class LogsViewModelTest {
    @get:Rule val folder = TemporaryFolder()

    private fun output(content: String) = NetProxyCtlOutput(true,
        listOf("""{"schema":1,"ok":true,"code":"test","data":{"content":"$content"}}"""), emptyList())

    @Test fun clearInvalidatesReadsStartedDuringTheWrite() = runBlocking {
        val clearStarted = CompletableDeferred<Unit>()
        val clearFinish = CompletableDeferred<Unit>()
        val readStarted = CompletableDeferred<Unit>()
        val readFinish = CompletableDeferred<Unit>()
        val scope = CoroutineScope(coroutineContext + SupervisorJob())
        try {
            val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
                if (args[1] == "clear") {
                    clearStarted.complete(Unit); clearFinish.await()
                } else {
                    readStarted.complete(Unit)
                    withContext(NonCancellable) { readFinish.await() }
                }
                output("obsolete")
            })
            val vm = LogsViewModel(LogRepository(client, folder.root), scope)
            val done = CompletableDeferred<Boolean>()
            vm.clear(LogType.KERNEL) { done.complete(it) }
            clearStarted.await()
            vm.refresh(LogType.KERNEL)
            readStarted.await()
            clearFinish.complete(Unit)
            assertTrue(done.await())
            readFinish.complete(Unit)
            scope.coroutineContext.job.children.toList().joinAll()
            assertTrue(vm.state.value.kernelLogs.isEmpty())
            assertEquals("", vm.state.value.error)
        } finally { scope.cancel() }
    }

    @Test fun onlyTheLatestRefreshPublishes() = runBlocking {
        val firstStarted = CompletableDeferred<Unit>()
        val firstFinish = CompletableDeferred<Unit>()
        val scope = CoroutineScope(coroutineContext + SupervisorJob())
        var calls = 0
        try {
            val client = NetProxyCtlClient(transport = NetProxyCtlTransport { _, _ ->
                if (++calls == 1) {
                    firstStarted.complete(Unit)
                    withContext(NonCancellable) { firstFinish.await() }
                    output("obsolete")
                } else output("latest")
            })
            val vm = LogsViewModel(LogRepository(client, folder.root), scope)
            vm.refresh(LogType.KERNEL)
            firstStarted.await()
            vm.refresh(LogType.KERNEL)
            withTimeout(2_000) {
                while (vm.state.value.kernelLogs.isEmpty()) yield()
            }
            firstFinish.complete(Unit)
            scope.coroutineContext.job.children.toList().joinAll()
            assertEquals("latest", vm.state.value.kernelLogs.single().rawLine)
            assertEquals("", vm.state.value.error)
        } finally { scope.cancel() }
    }
}
