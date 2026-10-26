package com.fanjv.netproxy.feature.logs.data

import com.fanjv.netproxy.core.api.*
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.launch
import kotlinx.coroutines.cancelAndJoin
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class LogRepositoryTest {
    @get:Rule val folder = TemporaryFolder()

    private fun client(write: suspend (File) -> Unit) = ModuleApiClient(object : TestApiTransport() {
        override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse = error("Unexpected JSON request")
        override suspend fun download(path: String, query: Map<String, String>, destination: File) {
            assertEquals("/diagnostics/export", path)
            assertEquals(setOf("manager_version", "manager_version_code"), query.keys)
            write(destination)
        }
    })

    @Test fun removesFailedOrEmptyExports() = runBlocking {
        for (write in listOf<suspend (File) -> Unit>({}, { error("export failed") })) {
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

    @Test fun cancelledDownloadRemovesPartialReport() = runBlocking {
        val started = CompletableDeferred<File>()
        val client = client { file ->
            file.writeBytes(byteArrayOf(1))
            started.complete(file)
            CompletableDeferred<Unit>().await()
        }
        val export = launch { LogRepository(client, folder.root).createReport() }
        val file = started.await()
        assertTrue(file.exists())
        export.cancelAndJoin()
        assertFalse(file.exists())
        assertTrue(folder.root.listFiles()!!.isEmpty())
    }

    @Test fun oversizedDownloadPropagatesThroughClientAndRemovesOnlyPartialReport() = runBlocking {
        val unrelated = folder.newFile("keep.txt").apply { writeText("keep") }
        val failure = ModuleApiException("api.body_too_large", "Diagnostic archive exceeds 64 MiB")
        val repository = LogRepository(client { file ->
            file.writeBytes(byteArrayOf(1, 2, 3))
            throw failure
        }, folder.root)
        val error = runCatching { repository.createReport() }.exceptionOrNull()
        assertSame(failure, error)
        assertEquals("api.body_too_large", (error as ModuleApiException).resultCode)
        assertEquals(listOf(unrelated), folder.root.listFiles()!!.toList())
        assertEquals("keep", unrelated.readText())
    }
}
