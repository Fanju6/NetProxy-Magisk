package com.fanjv.netproxy.bridge

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.CancellationSignal
import android.os.Process
import android.provider.OpenableColumns
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeout
import java.io.Closeable
import java.util.concurrent.atomic.AtomicReference
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

internal class AndroidDocuments(private val context: Context) {
    suspend fun read(uri: Uri): ImportContent = withTimeout(15_000) {
        if (uri.scheme != "content" || uri.authority.isNullOrBlank()) throw BridgeFailure("api.import_invalid")
        // An exported share Activity must not become a deputy for its own private Providers.
        val provider = context.packageManager.resolveContentProvider(uri.authority!!, 0)
        if (provider?.applicationInfo?.uid == Process.myUid() ||
            context.checkUriPermission(uri, Process.myPid(), Process.myUid(), Intent.FLAG_GRANT_READ_URI_PERMISSION) != PackageManager.PERMISSION_GRANTED)
            throw BridgeFailure("api.import_unreadable")
        suspendCancellableCoroutine { continuation ->
            val signal = CancellationSignal()
            val opened = AtomicReference<Closeable?>()
            continuation.invokeOnCancellation {
                signal.cancel()
                runCatching { opened.getAndSet(null)?.close() }
            }
            Dispatchers.IO.dispatch(EmptyCoroutineContext, Runnable {
                try {
                    if (!continuation.isActive) return@Runnable
                    val resolver = context.contentResolver
                    var name = ""
                    resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null, signal)?.use { cursor ->
                        if (cursor.moveToFirst()) {
                            val sizeColumn = cursor.getColumnIndex(OpenableColumns.SIZE)
                            if (sizeColumn >= 0 && !cursor.isNull(sizeColumn) && cursor.getLong(sizeColumn) > IMPORT_LIMIT)
                                throw BridgeFailure("api.body_too_large")
                            val nameColumn = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                            if (nameColumn >= 0) name = cursor.getString(nameColumn) ?: ""
                        }
                    }
                    val descriptor = resolver.openAssetFileDescriptor(uri, "r", signal)
                        ?: throw BridgeFailure("api.import_unreadable")
                    opened.set(descriptor)
                    if (!continuation.isActive) { descriptor.close(); return@Runnable }
                    val value = descriptor.use { asset ->
                        asset.createInputStream().use { input ->
                            opened.set(input)
                            if (!continuation.isActive) throw BridgeFailure("api.cancelled")
                            readImportContent(name, input)
                        }
                    }
                    if (continuation.isActive) continuation.resume(value)
                } catch (error: Exception) {
                    if (continuation.isActive) continuation.resumeWithException(
                        error as? BridgeFailure ?: BridgeFailure("api.import_unreadable"))
                } finally { runCatching { opened.getAndSet(null)?.close() } }
            })
        }
    }
}
