package com.fanjv.netproxy.core.command

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.cancel
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.ExperimentalCoroutinesApi
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ConfigurationWritesTest {
    @Test fun queuedWriteIsRegisteredBeforeItStarts() = runTest {
        val writes = ConfigurationWrites(this)
        var started = false
        val release = CompletableDeferred<Unit>()
        writes.launch("inbound/app", write = { started = true; release.await() })
        assertFalse(started)
        val related = async { writes.await("inbound/tun") }
        val unrelated = async { writes.await("singbox/config.json") }
        runCurrent()
        assertTrue(started)
        assertFalse(related.isCompleted)
        assertTrue(unrelated.isCompleted)
        release.complete(Unit)
        related.await()
    }

    @Test fun readWaitsForAllRelatedWritesIncludingNewlyQueuedOnes() = runTest {
        val writes = ConfigurationWrites(this)
        val first = CompletableDeferred<Unit>()
        val second = CompletableDeferred<Unit>()
        writes.launch("inbound/app", write = { first.await() })
        val reader = async { writes.await("inbound") }
        runCurrent()
        writes.launch("inbound/ebpf", write = { second.await() })
        first.complete(Unit)
        runCurrent()
        assertFalse(reader.isCompleted)
        second.complete(Unit)
        reader.await()
    }

    @Test fun postWriteReadDoesNotWaitForItsOwnConfirmation() = runTest {
        val writes = ConfigurationWrites(this)
        var confirmed = false
        writes.launch("inbound/backend", write = {}, afterWrite = {
            writes.await("inbound/tun")
            confirmed = true
        }).join()
        assertTrue(confirmed)
    }

    @Test fun cancellingReaderDoesNotCancelWrite() = runTest {
        val writes = ConfigurationWrites(this)
        val release = CompletableDeferred<Unit>()
        var saved = false
        val writer = writes.launch("module", write = { release.await(); saved = true })
        val reader = async { writes.await("module") }
        runCurrent()
        reader.cancelAndJoin()
        assertTrue(writer.isActive)
        release.complete(Unit)
        writer.join()
        assertTrue(saved)
    }

    @Test fun failureReleasesReadersAndReportsOnceAfterCleanup() = runTest {
        val errors = mutableListOf<String>()
        val writes = ConfigurationWrites(this) { errors += it.message.orEmpty() }
        var recovered = false
        writes.launch("inbound/backend", write = { error("save failed") }, afterWrite = {
            writes.await("inbound")
            recovered = true
        }).join()
        writes.await("inbound")
        assertTrue(recovered)
        assertEquals(listOf("save failed"), errors)
    }

    @Test fun confirmationFailureNotifiesDomainAndApplication() = runTest {
        val errors = mutableListOf<String>()
        val writes = ConfigurationWrites(this) { errors += it.message.orEmpty() }
        var failed = false
        writes.launch("inbound", write = {}, afterWrite = { error("status failed") },
            onFailure = { failed = true }).join()
        assertTrue(failed)
        assertEquals(listOf("status failed"), errors)
        writes.await("inbound")
    }

    @Test fun cancelledScopeDoesNotLeaveAnUnstartedWriteRegistered() = runTest {
        val scope = CoroutineScope(coroutineContext + Job())
        val writes = ConfigurationWrites(scope)
        scope.cancel()
        writes.launch("module", write = { fail("已取消的任务不能开始写入") }).join()
        writes.await("module")
    }
}
