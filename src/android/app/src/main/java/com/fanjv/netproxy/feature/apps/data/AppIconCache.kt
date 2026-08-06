package com.fanjv.netproxy.feature.apps.data

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.drawable.BitmapDrawable
import android.graphics.drawable.Drawable
import android.os.UserHandle
import androidx.collection.LruCache
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.core.graphics.createBitmap
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.withContext
import java.util.concurrent.Executors

/** Loads and caches application icons without blocking the Compose thread. */
object AppIconCache {
    private val lruCache: LruCache<String, ImageBitmap>
    private val dispatcher: CoroutineDispatcher

    init {
        val maxMemory = Runtime.getRuntime().maxMemory() / 1024
        val availableCacheSize = (maxMemory / 8).toInt()
        lruCache = object : LruCache<String, ImageBitmap>(availableCacheSize) {
            override fun sizeOf(key: String, value: ImageBitmap): Int {
                return (value.width * value.height * 4) / 1024
            }
        }

        val threadCount = (Runtime.getRuntime().availableProcessors() / 2).coerceIn(1, 4)
        dispatcher = Executors.newFixedThreadPool(threadCount).asCoroutineDispatcher()
    }

    private fun cacheKey(userId: String, packageName: String) = "$userId:$packageName"

    suspend fun loadIcon(
        context: Context,
        packageName: String,
        userId: String,
    ): ImageBitmap? {
        val key = cacheKey(userId, packageName)
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
                val imageBitmap = drawable.toBitmap().asImageBitmap()
                lruCache.put(key, imageBitmap)
                imageBitmap
            } catch (_: Exception) {
                null
            }
        }
    }

    private fun Drawable.toBitmap(): Bitmap {
        if (this is BitmapDrawable && bitmap != null) return bitmap

        val bitmap = if (intrinsicWidth <= 0 || intrinsicHeight <= 0) {
            createBitmap(1, 1)
        } else {
            createBitmap(intrinsicWidth, intrinsicHeight)
        }
        val canvas = Canvas(bitmap)
        setBounds(0, 0, canvas.width, canvas.height)
        draw(canvas)
        return bitmap
    }
}
