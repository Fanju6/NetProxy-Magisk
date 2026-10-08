package com.fanjv.netproxy.core.command

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/** 为 netproxyctl 创建短生命周期输入文件，并保证调用结束后清理。 */
internal class CommandFileStore(private val cacheDir: File) {

    suspend fun <T> withTextFile(
        prefix: String,
        suffix: String,
        content: String,
        block: suspend (File) -> T
    ): T {
        return withCommandFile(cacheDir, prefix, suffix, { it.writeText(content, Charsets.UTF_8) }, block)
    }
}

internal suspend fun <T> withCommandFile(
    directory: File,
    prefix: String,
    suffix: String,
    write: (File) -> Unit,
    block: suspend (File) -> T
): T = withContext(Dispatchers.IO) {
    val file = File.createTempFile(prefix, suffix, directory)
    try {
        write(file)
        block(file)
    } finally {
        // 删除与创建处于同一 IO 生命周期，不经过可取消的文件返回边界。
        file.delete()
    }
}
