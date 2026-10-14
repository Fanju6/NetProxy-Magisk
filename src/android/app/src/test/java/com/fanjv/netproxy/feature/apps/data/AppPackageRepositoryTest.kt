package com.fanjv.netproxy.feature.apps.data

import com.fanjv.netproxy.core.api.*
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.*
import org.junit.Test

class AppPackageRepositoryTest {
    private val iconKey = "a".repeat(64)
    private fun user(name: String, id: Int = 0, label: String = name) =
        """{"id":$id,"name":"User $id","revision":"apps-$id","apps":[{"package_name":"$name","label":"$label","uid":${id * 100000 + 10001},"system":false,"version_code":1,"last_update_time":1,"icon_key":"$iconKey"}]}"""
    private fun listing(name: String) = apiSuccess("""{"users":[${user(name)}]}""")
    private fun AndroidAppsSnapshot.packages() = users.flatMap { it.apps }.map { it.packageName }

    @Test fun invalidationReloadsOneSnapshotForAllUsersAndApplications() = runBlocking {
        val calls = AtomicInteger()
        val repository = AppPackageRepository(ModuleApiClient(TestApiTransport { request ->
            calls.incrementAndGet()
            assertEquals("GET", request.method)
            assertEquals("/android/apps", request.path)
            assertTrue(request.query.isEmpty())
            listing("example.app")
        }))
        val first = repository.snapshot()
        assertSame(first, repository.snapshot())
        assertEquals(1, calls.get())
        repository.invalidatePackageListingCaches()
        assertNotSame(first, repository.snapshot())
        assertEquals(2, calls.get())
    }

    @Test fun concurrentReadersShareOneCompleteSnapshot() = runBlocking {
        val calls = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val repository = AppPackageRepository(ModuleApiClient(TestApiTransport {
            calls.incrementAndGet()
            entered.complete(Unit)
            withTimeout(5000) { release.await() }
            apiSuccess("""{"users":[${user("example.app")},${user("example.app", 10)}]}""")
        }))
        val readers = List(16) { async { repository.snapshot() } }
        withTimeout(5000) { entered.await() }
        release.complete(Unit)
        val snapshots = readers.awaitAll()
        assertEquals(1, calls.get())
        snapshots.forEach { assertSame(snapshots.first(), it) }
        assertEquals(listOf(0, 10), snapshots.first().users.map { it.id })
    }

    @Test fun oldListingCannotRefillCacheAfterInvalidation() = runBlocking {
        val calls = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val repository = AppPackageRepository(ModuleApiClient(TestApiTransport {
            if (calls.incrementAndGet() == 1) {
                entered.complete(Unit); withTimeout(5000) { release.await() }; listing("old.example")
            } else listing("new.example")
        }))
        val old = async { repository.snapshot().packages() }
        withTimeout(5000) { entered.await() }
        repository.invalidatePackageListingCaches()
        assertEquals(listOf("new.example"), repository.snapshot().packages())
        release.complete(Unit)
        assertEquals(listOf("old.example"), old.await())
        assertEquals(listOf("new.example"), repository.snapshot().packages())
        assertEquals(2, calls.get())
    }

    @Test fun samePackageUsesEachUsersLabelAndRevision() = runBlocking {
        val repository = AppPackageRepository(ModuleApiClient(TestApiTransport {
            apiSuccess("""{"users":[${user("example.app", 0, "Owner app")},${user("example.app", 10, "Work app")}]}""")
        }))
        val snapshot = repository.snapshot()
        assertEquals(listOf("Owner app", "Work app"), snapshot.users.map { it.apps.single().label })
        assertEquals(listOf("User 0", "User 10"), snapshot.users.map { it.name })
        assertEquals(listOf("apps-0", "apps-10"), snapshot.users.map { it.revision })
    }

    @Test fun failureDoesNotInventOwnerOrCacheEmptyApplications() = runBlocking {
        val calls = AtomicInteger()
        val repository = AppPackageRepository(ModuleApiClient(TestApiTransport {
            calls.incrementAndGet()
            throw ModuleApiException("android.unavailable", "fixture")
        }))
        repeat(2) {
            try { repository.snapshot(); fail("expected query failure") } catch (error: ModuleApiException) {
                assertEquals("android.unavailable", error.resultCode)
            }
        }
        assertEquals(2, calls.get())
    }

    @Test fun cancellationDoesNotPopulateSnapshotAndNextReadRetries() = runBlocking {
        val calls = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val repository = AppPackageRepository(ModuleApiClient(TestApiTransport {
            if (calls.incrementAndGet() == 1) {
                entered.complete(Unit)
                CompletableDeferred<Unit>().await()
            }
            listing("example.app")
        }))
        val cancelled = async { repository.snapshot() }
        withTimeout(5000) { entered.await() }
        cancelled.cancel()
        cancelled.join()
        assertEquals(listOf("example.app"), repository.snapshot().packages())
        assertEquals(2, calls.get())
    }

    @Test fun iconsUseUnchangedHashWithBinaryTransport() = runBlocking {
        val png = byteArrayOf(1, 2, 3)
        val transport = object : TestApiTransport() {
            override suspend fun respond(request: ApiTestRequest): ModuleHttpResponse {
                assertEquals("/android/apps", request.path)
                assertTrue(request.query.isEmpty())
                return listing("example.app")
            }
            override suspend fun image(path: String): ByteArray {
                assertEquals("/android/icons/$iconKey", path)
                return png
            }
        }
        val repository = AppPackageRepository(ModuleApiClient(transport))
        val key = repository.snapshot().users.single().apps.single().iconKey
        assertEquals(iconKey, key)
        assertArrayEquals(png, repository.icon(key))
    }
}
