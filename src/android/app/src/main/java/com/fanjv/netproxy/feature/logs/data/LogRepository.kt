package com.fanjv.netproxy.feature.logs.data

import android.content.Context
import com.fanjv.netproxy.BuildConfig
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File
import java.io.IOException

/** 日志读取、清理和诊断包导出的模块数据入口。 */
internal class LogRepository(
    private val client: NetProxyCtlClient,
    context: Context
) {
    private val reportsDir = File(context.applicationContext.cacheDir, "reports")

    suspend fun read(type: LogType, lines: Int = 800): List<LogItem> {
        val content = client.execute("logs", "show", type.commandName, lines.toString())
            .data.jsonObject["content"]?.jsonPrimitive?.content.orEmpty()
        return LogParser.parse(content, type)
    }

    suspend fun clear(type: LogType) {
        client.execute("logs", "clear", type.commandName)
    }

    suspend fun export(outputPath: String): String =
        client.execute(
            "logs", "export",
            "--manager-version", BuildConfig.VERSION_NAME,
            "--manager-version-code", BuildConfig.VERSION_CODE.toString(),
            outputPath,
        ).data.jsonObject["path"]
            ?.jsonPrimitive?.content ?: outputPath

    suspend fun createReport(): File {
        val target = withContext(Dispatchers.IO) {
            reportsDir.mkdirs()
            File(reportsDir, "NetProxy_Logs_${System.currentTimeMillis()}.tar.gz").also {
                it.delete()
                check(it.createNewFile()) { "无法创建诊断包临时文件" }
            }
        }
        export(target.absolutePath)
        withContext(Dispatchers.IO) {
            if (!target.isFile || target.length() == 0L) {
                throw IOException("诊断包导出后为空")
            }
        }
        return target
    }

    private val LogType.commandName: String
        get() = when (this) {
            LogType.SERVICE -> "service"
            LogType.KERNEL -> "core"
        }
}
