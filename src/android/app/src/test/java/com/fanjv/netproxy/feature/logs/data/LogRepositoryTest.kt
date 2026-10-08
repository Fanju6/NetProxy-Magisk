package com.fanjv.netproxy.feature.logs.data

import com.fanjv.netproxy.core.command.*
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.launch
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class LogRepositoryTest {
    @get:Rule val folder = TemporaryFolder()

    private fun client(write: (File) -> Unit) = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
        write(File(args.last()))
        NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"code":"test","data":{}}"""), emptyList())
    })

    @Test fun removesFailedOrEmptyExports() = runBlocking {
        for (write in listOf<(File) -> Unit>({}, { error("export failed") })) {
            try { LogRepository(client(write), folder.root).createReport(); fail("expected export failure") }
            catch (_: Exception) {}
            assertTrue(folder.root.listFiles()!!.isEmpty())
        }
    }

    @Test fun keepsUniqueReportsAndExpiresOnlyOwnedOldFiles() = runBlocking {
        val old = folder.newFile("NetProxy_Logs_old.tar.gz").apply { setLastModified(1) }
        val unrelated = folder.newFile("unrelated.txt").apply { setLastModified(1) }
        val repository = LogRepository(client { it.writeBytes(byteArrayOf(1, 2, 3)) }, folder.root)
        val first = repository.createReport()
        val second = repository.createReport()
        assertNotEquals(first, second)
        assertEquals(3L, first.length())
        assertTrue(first.isFile && second.isFile)
        assertFalse(old.exists())
        assertTrue(unrelated.exists())
    }

    @Test fun cancelledExportFinishesBeforeDeletingTheReport() = runBlocking {
        val started = CompletableDeferred<File>()
        val complete = CompletableDeferred<Unit>()
        val client = NetProxyCtlClient(transport = NetProxyCtlTransport { args, _ ->
            val file = File(args.last())
            started.complete(file)
            complete.await()
            assertTrue(file.exists())
            file.writeBytes(byteArrayOf(1))
            NetProxyCtlOutput(true, listOf("""{"schema":1,"ok":true,"data":{}}"""), emptyList())
        })
        val export = launch { LogRepository(client, folder.root).createReport() }
        val file = started.await()
        export.cancel()
        assertTrue(file.exists())
        complete.complete(Unit)
        export.join()
        assertTrue(folder.root.listFiles()!!.isEmpty())
    }
}
