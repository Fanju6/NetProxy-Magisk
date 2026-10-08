package com.fanjv.netproxy.core.command

import com.topjohnwu.superuser.Shell
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonPrimitive
import kotlin.coroutines.resume

/** 安全构造 root 命令；业务操作只允许调用 netproxyctl 或 Android 的 pm。 */
internal object ShellCommand {
    suspend fun execute(vararg args: String, isolated: Boolean = false): Shell.Result {
        val command = args.joinToString(" ", transform = ::quote)
        if (isolated) return withContext(Dispatchers.IO) {
            Shell.Builder.create().setFlags(Shell.FLAG_MOUNT_MASTER).setTimeout(10).build().use { shell ->
                check(shell.isRoot) { "未获得 Root 权限" }
                await(shell.newJob().add(command).to(ArrayList(), ArrayList()))
            }
        }
        return await(Shell.cmd(command))
    }

    private suspend fun await(job: Shell.Job): Shell.Result = suspendCancellableCoroutine { continuation ->
        job.submit(null) { result -> continuation.resume(result) }
    }

    private fun quote(value: String): String =
        "'" + value.replace("'", "'\"'\"'") + "'"
}

/** 纯内存解析和更新 KEY=value 配置，实际读写由 netproxyctl 事务完成。 */
internal object ShellConfigFile {
    fun boolean(value: String?, default: Boolean = false): Boolean = when (value) {
        "1", "true" -> true
        "0", "false" -> false
        null -> default
        else -> error("布尔配置无效: $value")
    }

    fun parse(content: String): Map<String, String> = buildMap {
        content.lineSequence().forEach { rawLine ->
            val line = rawLine.trim()
            if (line.isEmpty() || line.startsWith('#')) return@forEach
            val separator = line.indexOf('=')
            if (separator <= 0) return@forEach

            val key = line.substring(0, separator).trim()
            check(key !in this) { "配置键重复: $key" }
            put(key, decodeValue(line.substring(separator + 1).trim()))
        }
    }

    fun updateValue(
        content: String,
        key: String,
        value: String,
        forceQuotes: Boolean = false
    ): String {
        require(key.matches(Regex("[A-Z][A-Z0-9_]*"))) { "配置键无效" }
        require(value.none { it in "\n\r\t" || it.code == 0 }) { "配置值无效" }
        val formatted = if (forceQuotes || value.isBlank() || value.any { it.isWhitespace() || it in "\"\\" || it.code < 32 }) {
            JsonPrimitive(value).toString()
        } else {
            value
        }
        val lines = content.split('\n').toMutableList()
        val prefix = "$key="
        val index = lines.indexOfFirst { line ->
            !line.trimStart().startsWith('#') && line.substringBefore('=').trim() == key && '=' in line
        }
        if (index >= 0) {
            lines[index] = prefix + formatted
        } else {
            lines += prefix + formatted
        }
        return lines.joinToString("\n")
    }

    private fun decodeValue(value: String): String {
        if (!value.startsWith('"')) {
            require(value.none { it in "\r\n\t" }) { "不能包含换行或制表符" }
            return value
        }
        require(value.length >= 2 && value.endsWith('"')) { "双引号未闭合" }
        // Go 的 strconv.Unquote 还接受十六进制、八进制和八位 Unicode 转义。
        val bytes = java.io.ByteArrayOutputStream()
        var index = 1
        while (index < value.lastIndex) {
            val start = index
            if (value[index++] != '\\') {
                require(value[start] != '"' && value[start] !in "\r\n") { "字符串无效" }
                while (index < value.lastIndex && value[index] != '\\' && value[index] != '"') index++
                bytes.write(value.substring(start, index).toByteArray(Charsets.UTF_8))
                continue
            }
            require(index < value.lastIndex) { "转义未完成" }
            val escape = value[index++]
            val simple = when (escape) {
                'a' -> 7; 'b' -> 8; 'f' -> 12; 'n' -> 10; 'r' -> 13; 't' -> 9; 'v' -> 11
                '\\' -> 92; '"' -> 34
                else -> null
            }
            if (simple != null) { bytes.write(simple); continue }
            val digits = when (escape) { 'x' -> 2; 'u' -> 4; 'U' -> 8; in '0'..'7' -> 3; else -> throw IllegalArgumentException("转义无效") }
            val octal = escape in '0'..'7'
            val from = if (octal) index - 1 else index
            require(from + digits <= value.lastIndex) { "转义未完成" }
            val encoded = value.substring(from, from + digits)
            require(encoded.all { if (octal) it in '0'..'7' else it in '0'..'9' || it in 'a'..'f' || it in 'A'..'F' }) { "转义无效" }
            val code = encoded.toLong(if (octal) 8 else 16)
            index = from + digits
            if (octal || escape == 'x') {
                require(code <= 255) { "字节转义无效" }
                bytes.write(code.toInt())
            } else {
                require(code <= 0x10ffff && code !in 0xd800..0xdfff) { "Unicode 转义无效" }
                bytes.write(String(Character.toChars(code.toInt())).toByteArray(Charsets.UTF_8))
            }
        }
        val result = Charsets.UTF_8.newDecoder().decode(java.nio.ByteBuffer.wrap(bytes.toByteArray())).toString()
        require(result.none { it in "\r\n\t" }) { "不能包含换行或制表符" }
        return result
    }
}
