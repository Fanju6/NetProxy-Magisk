package com.fanjv.netproxy.core.command

import com.topjohnwu.superuser.Shell
import java.util.concurrent.TimeUnit
import java.util.concurrent.TimeoutException

/** 安全构造 root 命令；业务操作只允许调用 netproxyctl 或 Android 的 pm。 */
internal object ShellCommand {
    fun exec(vararg args: String): Shell.Result =
        Shell.cmd(args.joinToString(" ", transform = ::quote)).exec()

    fun exec(timeoutMillis: Long, vararg args: String): TimedShellResult {
        val future = Shell.cmd(args.joinToString(" ", transform = ::quote)).enqueue()
        return try {
            val result = future.get(timeoutMillis.coerceAtLeast(1L), TimeUnit.MILLISECONDS)
            TimedShellResult(result.isSuccess, result.out, result.err)
        } catch (_: TimeoutException) {
            future.cancel(true)
            TimedShellResult(false, emptyList(), listOf("命令执行超时"))
        } catch (_: InterruptedException) {
            future.cancel(true)
            Thread.currentThread().interrupt()
            TimedShellResult(false, emptyList(), listOf("命令执行被中断"))
        } catch (error: Exception) {
            future.cancel(true)
            TimedShellResult(false, emptyList(), listOf(error.message ?: "命令执行失败"))
        }
    }

    private fun quote(value: String): String =
        "'" + value.replace("'", "'\"'\"'") + "'"
}

internal data class TimedShellResult(
    val successful: Boolean,
    val stdout: List<String>,
    val stderr: List<String>
)

/** 纯内存解析和更新 KEY=value 配置，实际读写由 netproxyctl 事务完成。 */
internal object ShellConfigFile {
    fun parse(content: String): Map<String, String> = buildMap {
        content.lineSequence().forEach { rawLine ->
            val line = rawLine.trim()
            if (line.isEmpty() || line.startsWith('#')) return@forEach
            val separator = line.indexOf('=')
            if (separator <= 0) return@forEach

            val key = line.substring(0, separator).trim()
            var value = line.substring(separator + 1).trim()
            if (value.length >= 2 &&
                ((value.first() == '"' && value.last() == '"') ||
                        (value.first() == '\'' && value.last() == '\''))
            ) {
                value = value.substring(1, value.lastIndex)
            }
            put(key, value)
        }
    }

    fun updateValue(
        content: String,
        key: String,
        value: String,
        forceQuotes: Boolean = false
    ): String {
        require(key.matches(Regex("[A-Z][A-Z0-9_]*"))) { "配置键无效" }
        require(value.none { it == '\n' || it == '\r' || it.code == 0 }) { "配置值无效" }
        val escaped = value.replace("\\", "\\\\").replace("\"", "\\\"")
        val formatted = if (forceQuotes || value.isBlank() || value.any(Char::isWhitespace)) {
            "\"$escaped\""
        } else {
            escaped
        }
        val lines = content.split('\n').toMutableList()
        val prefix = "$key="
        val index = lines.indexOfFirst { it.startsWith(prefix) }
        if (index >= 0) {
            lines[index] = prefix + formatted
        } else {
            lines += prefix + formatted
        }
        return lines.joinToString("\n")
    }
}
