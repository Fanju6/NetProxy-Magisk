package com.fanjv.netproxy.core.api

import kotlinx.serialization.json.Json
import org.junit.Assert.*
import org.junit.Test
import java.io.IOException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

class ModuleConnectionTest {
    private val connection = ModuleConnection("https://127.0.0.1:9091", "a".repeat(64), "b".repeat(64))

    @Test fun readsAndValidatesDiskOnlyOnce() {
        val reads = AtomicInteger()
        val store = ConnectionStore({ reads.incrementAndGet(); Json.encodeToString(connection) }, {})
        val first = store.read()
        repeat(100) { assertSame(first, store.read()) }
        assertEquals(1, reads.get())
    }

    @Test fun cachesMissingCredentialsAndDoesNotReloadInvalidatedDisk() {
        val reads = AtomicInteger()
        val store = ConnectionStore({ reads.incrementAndGet(); Json.encodeToString(connection) }, {})
        val first = requireNotNull(store.read())
        assertTrue(store.invalidate(first))
        repeat(10) { assertNull(store.read()) }
        assertEquals(1, reads.get())
        val missing = ConnectionStore({ reads.incrementAndGet(); null }, {})
        repeat(10) { assertNull(missing.read()) }
        assertEquals(2, reads.get())
    }

    @Test fun providerWriteImmediatelyPublishesNewCredentials() {
        var persisted = ""
        var reads = 0
        val store = ConnectionStore({ reads++; null }, { persisted = it })
        store.write(Json.encodeToString(connection))
        assertEquals(connection, store.read())
        assertEquals(connection, Json.decodeFromString<ModuleConnection>(persisted))
        assertEquals(0, reads)
    }

    @Test fun staleFailureCannotInvalidateNewOrRedeliveredCredentials() {
        val store = ConnectionStore({ Json.encodeToString(connection) }, {})
        val old = requireNotNull(store.read())
        store.write(Json.encodeToString(connection))
        val delivered = requireNotNull(store.read())
        assertNotSame(old, delivered)
        assertFalse(store.invalidate(old))
        assertSame(delivered, store.read())
        store.write(Json.encodeToString(connection.copy(secret = "c".repeat(64))))
        assertFalse(store.invalidate(delivered))
        assertEquals("c".repeat(64), store.read()?.secret)
    }

    @Test fun failedPersistenceKeepsLastConfirmedCache() {
        val store = ConnectionStore({ Json.encodeToString(connection) }, { throw IOException("test failure") })
        val old = store.read()
        assertThrows(IOException::class.java) { store.write(Json.encodeToString(connection.copy(secret = "c".repeat(64)))) }
        assertSame(old, store.read())
    }

    @Test fun parallelFirstReadsShareOneSnapshot() {
        val reads = AtomicInteger()
        val store = ConnectionStore({ reads.incrementAndGet(); Json.encodeToString(connection) }, {})
        val executor = Executors.newFixedThreadPool(8)
        val start = CountDownLatch(1)
        try {
            val tasks = (1..8).map { executor.submit<ModuleConnection?> { start.await(); store.read() } }
            start.countDown()
            val snapshots = tasks.map { it.get(3, TimeUnit.SECONDS) }
            snapshots.forEach { assertSame(snapshots.first(), it) }
            assertEquals(1, reads.get())
        } finally { executor.shutdownNow() }
    }

    @Test fun rejectsMalformedSecretsAndNonLoopbackAddresses() {
        listOf("http://127.0.0.1:9091", "https://localhost:9091", "https://192.0.2.1:9091",
            "https://127.0.0.1:9091/", "https://user@127.0.0.1:9091", "https://127.0.0.1:9091?secret=x").forEach {
            assertThrows(IllegalArgumentException::class.java) { connection.copy(address = it).validate() }
        }
        assertThrows(IllegalArgumentException::class.java) { connection.copy(secret = "bad").validate() }
        assertThrows(IllegalArgumentException::class.java) { connection.copy(certificate_sha256 = "B".repeat(64)).validate() }
        assertEquals(connection.copy(address = "https://[::1]:9091"), connection.copy(address = "https://[::1]:9091").validate())
    }
}
