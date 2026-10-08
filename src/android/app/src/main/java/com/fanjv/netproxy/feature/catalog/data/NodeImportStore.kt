package com.fanjv.netproxy.feature.catalog.data

import android.content.Context
import android.net.Uri
import com.fanjv.netproxy.core.command.withCommandFile
import java.io.File

/** 将 Android 文档 URI 暂存为 netproxyctl 可读取的短生命周期文件。 */
internal class NodeImportStore(context: Context) {
    private val contentResolver = context.applicationContext.contentResolver
    private val cacheDir = context.applicationContext.cacheDir

    suspend fun <T> withImportedFile(uri: Uri, block: suspend (File) -> T): T {
        return withCommandFile(cacheDir, "netproxy-import-", ".txt", { target ->
            contentResolver.openInputStream(uri)?.use { input ->
                target.outputStream().use(input::copyTo)
            } ?: error("无法读取所选文件")
        }, block)
    }
}

