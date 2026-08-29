package com.fanjv.netproxy.feature.apps.data

import android.content.ComponentCallbacks2
import android.content.Context
import android.graphics.Bitmap
import android.os.UserHandle
import androidx.collection.LruCache
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.core.graphics.drawable.toBitmap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** 按界面尺寸加载应用图标，避免缓存 Launcher 使用的原始大图。 */
object AppIconCache {
    private const val MAX_CACHE_KB = 8 * 1024
    private val dispatcher = Dispatchers.IO.limitedParallelism(2)
    private val lruCache = object : LruCache<String, ImageBitmap>(MAX_CACHE_KB) {
        override fun sizeOf(key: String, value: ImageBitmap): Int {
            val bytes = value.width.toLong() * value.height * 4
            return ((bytes + 1023) / 1024).coerceAtLeast(1).toInt()
        }
    }

    private fun cacheKey(userId: String, packageName: String, sizePx: Int) =
        "$userId:$packageName@$sizePx"

    suspend fun loadIcon(
        context: Context,
        packageName: String,
        userId: String,
        sizePx: Int,
    ): ImageBitmap? {
        val targetSize = sizePx.coerceAtLeast(1)
        val key = cacheKey(userId, packageName, targetSize)
        lruCache[key]?.let { return it }

        return withContext(dispatcher) {
            try {
                lruCache[key]?.let { return@withContext it }
                val packageManager = context.packageManager
                val info = packageManager.getApplicationInfo(packageName, 0)
                val baseIcon = packageManager.getApplicationIcon(info)
                val drawable = userId.toIntOrNull()
                    ?.takeIf { it != 0 }
                    ?.let {
                        packageManager.getUserBadgedIcon(
                            baseIcon,
                            UserHandle.getUserHandleForUid(it * 100_000)
                        )
                    }
                    ?: baseIcon
                val imageBitmap = drawable
                    .toBitmap(targetSize, targetSize, Bitmap.Config.ARGB_8888)
                    .asImageBitmap()
                lruCache.put(key, imageBitmap)
                imageBitmap
            } catch (_: Exception) {
                null
            }
        }
    }

    fun trimMemory(level: Int) {
        if (level >= ComponentCallbacks2.TRIM_MEMORY_UI_HIDDEN) lruCache.evictAll()
    }

    fun clear() = lruCache.evictAll()
}
