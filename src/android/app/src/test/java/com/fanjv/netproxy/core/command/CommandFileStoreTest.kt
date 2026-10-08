package com.fanjv.netproxy.core.command

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class CommandFileStoreTest {
    @get:Rule val folder = TemporaryFolder()

    @Test fun cleansUpAfterWriteFailure() = runBlocking {
        try {
            withCommandFile(folder.root, "fixture-", ".json", {
                it.writeText("private fixture")
                error("write failed")
            }) { fail("must not consume failed input") }
            fail("expected write failure")
        } catch (_: IllegalStateException) {}
        assertTrue(folder.root.listFiles()!!.isEmpty())
    }

    @Test fun cleansUpAfterConsumptionAndCancellation() = runBlocking {
        val store = CommandFileStore(folder.root)
        store.withTextFile("fixture-", ".json", "private fixture") {
            assertEquals("private fixture", it.readText())
        }
        val ready = CompletableDeferred<Unit>()
        val job = launch {
            store.withTextFile("fixture-", ".json", "private fixture") {
                ready.complete(Unit)
                awaitCancellation()
            }
        }
        ready.await()
        job.cancelAndJoin()
        assertTrue(folder.root.listFiles()!!.isEmpty())
    }

    @Test fun cancellationDuringCreationStillDeletesFile() = runBlocking {
        val written = CompletableDeferred<Unit>()
        val release = java.util.concurrent.CountDownLatch(1)
        val job = launch {
            withCommandFile(folder.root, "fixture-", ".json", {
                it.writeText("private fixture")
                written.complete(Unit)
                release.await()
            }) { kotlinx.coroutines.currentCoroutineContext().ensureActive() }
        }
        written.await()
        job.cancel()
        release.countDown()
        job.join()
        assertTrue(folder.root.listFiles()!!.isEmpty())
    }
}
