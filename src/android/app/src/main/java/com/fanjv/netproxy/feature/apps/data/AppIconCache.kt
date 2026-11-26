package com.fanjv.netproxy.feature.apps.data

import android.content.ComponentCallbacks2
import android.graphics.BitmapFactory
import androidx.collection.LruCache
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import kotlinx.coroutines.withContext

object AppIconCache {
    private const val MAX_CACHE_KB = 8 * 1024
    private val cacheLock = Any()
    var revision by mutableLongStateOf(0L)
        private set
    var generation by mutableLongStateOf(0L)
        private set
    private val lruCache = object : LruCache<String, ImageBitmap>(MAX_CACHE_KB) {
        override fun sizeOf(key: String, value: ImageBitmap): Int =
            ((value.width.toLong() * value.height * 4 + 1023) / 1024).coerceAtLeast(1).toInt()
    }

    fun cachedIcon(key: String): ImageBitmap? = synchronized(cacheLock) { lruCache[key] }

    internal suspend fun loadMissing(keys: Collection<String>, repository: AppPackageRepository) = coroutineScope {
        val expectedGeneration = generation
        val slots = Semaphore(4)
        keys.distinct().filter { it.isNotEmpty() && cachedIcon(it) == null }.map { key ->
            async {
                slots.withPermit {
                    try { putIcon(key, repository.icon(key), expectedGeneration) }
                    catch (error: CancellationException) { throw error }
                    catch (_: Exception) { }
                }
            }
        }.awaitAll()
    }

    suspend fun putIcon(key: String, bytes: ByteArray, expectedGeneration: Long) = withContext(Dispatchers.Default) {
        val image = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap() ?: return@withContext
        currentCoroutineContext().ensureActive()
        synchronized(cacheLock) {
            if (generation != expectedGeneration) return@synchronized
            lruCache.put(key, image)
            revision++
        }
    }

    fun trimMemory(level: Int) {
        if (level >= ComponentCallbacks2.TRIM_MEMORY_UI_HIDDEN) clear()
    }

    fun clear() = synchronized(cacheLock) {
        generation++
        revision++
        lruCache.evictAll()
    }
}
