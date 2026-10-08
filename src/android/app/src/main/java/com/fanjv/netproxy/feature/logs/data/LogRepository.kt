package com.fanjv.netproxy.feature.logs.data

import android.content.Context
import com.fanjv.netproxy.BuildConfig
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File
import java.io.IOException

/** 日志读取、清理和诊断包导出的模块数据入口。 */
internal class LogRepository(
    private val client: NetProxyCtlClient,
    private val reportsDir: File
) {
    constructor(client: NetProxyCtlClient, context: Context) :
        this(client, File(context.applicationContext.cacheDir, "reports"))

    suspend fun read(type: LogType, lines: Int = 800): List<LogItem> {
        val data = client.execute("logs", "show", type.commandName, lines.toString()).data.jsonObject
        return withContext(Dispatchers.Default) { when (type) {
            LogType.SERVICE -> {
                val entries = data["entries"]?.jsonArray
                    ?: error("Native 日志响应缺少 entries")
                LogParser.parseNative(entries)
            }

            LogType.KERNEL -> {
                val content = data["content"]?.jsonPrimitive?.content.orEmpty()
                LogParser.parseKernel(content)
            }
        } }
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
        var target: File? = null
        try {
            return withContext(Dispatchers.IO) { createReportFile().also { target = it } }
        } catch (error: Throwable) {
            target?.delete()
            throw error
        }
    }

    private suspend fun createReportFile(): File {
        check(reportsDir.mkdirs() || reportsDir.isDirectory) { "无法创建诊断包目录" }
        // 分享接收方可能稍后才读取，报告保留一天；保存副本由调用方立即删除。
        val cutoff = System.currentTimeMillis() - 24 * 60 * 60 * 1_000L
        reportsDir.listFiles()?.filter { it.name.startsWith("NetProxy_Logs_") && it.lastModified() < cutoff }
            ?.forEach(File::delete)
        val target = File.createTempFile("NetProxy_Logs_", ".tar.gz", reportsDir)
        try {
            export(target.absolutePath)
            if (!target.isFile || target.length() == 0L) {
                throw IOException("诊断包导出后为空")
            }
            return target
        } catch (error: Throwable) {
            target.delete()
            throw error
        }
    }

    private val LogType.commandName: String
        get() = when (this) {
            LogType.SERVICE -> "service"
            LogType.KERNEL -> "core"
        }
}
