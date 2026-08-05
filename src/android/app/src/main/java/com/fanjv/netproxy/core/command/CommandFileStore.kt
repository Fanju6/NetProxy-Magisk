package com.fanjv.netproxy.core.command

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/** 为 netproxyctl 创建短生命周期输入文件，并保证调用结束后清理。 */
internal class CommandFileStore(context: Context) {
    private val cacheDir = context.applicationContext.cacheDir

    suspend fun <T> withTextFile(
        prefix: String,
        suffix: String,
        content: String,
        block: suspend (File) -> T
    ): T {
        val file = withContext(Dispatchers.IO) {
            File.createTempFile(prefix, suffix, cacheDir).also {
                it.writeText(content, Charsets.UTF_8)
            }
        }
        return try {
            block(file)
        } finally {
            withContext(Dispatchers.IO) { file.delete() }
        }
    }
}
