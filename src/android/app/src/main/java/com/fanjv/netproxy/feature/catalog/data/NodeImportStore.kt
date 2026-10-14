package com.fanjv.netproxy.feature.catalog.data

import android.content.Context
import android.net.Uri
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction

/** 读取用户选中的文档内容，不创建模块输入文件。 */
internal class NodeImportStore(context: Context) {
    private val contentResolver = context.applicationContext.contentResolver
    suspend fun read(uri: Uri): String = withContext(Dispatchers.IO) {
        val content = contentResolver.openInputStream(uri)?.use { input ->
            val output = ByteArrayOutputStream()
            val buffer = ByteArray(8192)
            while (true) {
                currentCoroutineContext().ensureActive()
                val size = input.read(buffer)
                if (size < 0) break
                require(output.size() + size <= 32 * 1024 * 1024) { "导入文件过大" }
                output.write(buffer, 0, size)
            }
            output.toByteArray()
        } ?: error("无法读取所选文件")
        Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(content)).toString()
    }
}

