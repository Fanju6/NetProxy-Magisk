package com.fanjv.netproxy.feature.apps.data

import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.*
import org.junit.Test

class AppPackageRepositoryTest {
    @Test fun invalidationReloadsUsersAndPackageListings() = runBlocking {
        val calls = AtomicInteger()
        val repository = AppPackageRepository(queryPackages = { args ->
            calls.incrementAndGet()
            if (args.last() == "users") listOf("UserInfo{0:Owner:13}") else listOf("package:example.app")
        }, resolveLabel = { it })
        repeat(2) { repository.getUsers(); repository.getInstalledPackages() }
        assertEquals(2, calls.get())
        repository.invalidatePackageListingCaches()
        repository.getUsers(); repository.getInstalledPackages()
        assertEquals(4, calls.get())
    }

    @Test fun oldListingCannotRefillCacheAfterInvalidation() = runBlocking {
        val calls = AtomicInteger()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val repository = AppPackageRepository(queryPackages = {
            if (calls.incrementAndGet() == 1) {
                entered.complete(Unit); release.await(); listOf("package:old.example")
            } else listOf("package:new.example")
        }, resolveLabel = { it })
        val old = async { repository.getInstalledPackages() }
        withTimeout(5_000) { entered.await() }
        repository.invalidatePackageListingCaches()
        assertEquals(listOf("new.example"), repository.getInstalledPackages())
        release.complete(Unit)
        assertEquals(listOf("old.example"), old.await())
        assertEquals(listOf("new.example"), repository.getInstalledPackages())
        assertEquals(2, calls.get())
    }
}
