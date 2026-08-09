package com.fanjv.netproxy.feature.logs.data

import androidx.compose.runtime.Immutable
import java.util.regex.Pattern

/** 日志类型：服务、内核。 */
enum class LogType {
    SERVICE,
    KERNEL
}

/** 日志级别。 */
enum class LogLevel {
    DEBUG,
    INFO,
    WARN,
    ERROR,
    UNKNOWN
}

/** 一条出站连接的路由信息（来源 / 目标 / 出站）。 */
@Immutable
data class OutboundFlow(
    val source: String,
    val target: String,
    val outbound: String
)

/** 解析后的单条日志。 */
@Immutable
data class LogItem(
    val rawLine: String,
    val timestamp: String,
    val level: LogLevel,
    val tag: String,
    val message: String,
    val outboundFlow: OutboundFlow? = null
)

/** 将模块服务和内核日志转换为统一展示模型。 */
internal object LogParser {

    // 格式 1：[2026-05-31 18:09:49] [INFO] message
    private val serviceLogPattern1 =
        Pattern.compile("""^\[([\d\-:\s]+)]\s+\[([A-Za-z]+)]\s+(.*)$""")

    // 格式 2：[Info]: message 或 [Warn]: message
    private val serviceLogPattern2 = Pattern.compile("""^\[([A-Za-z]+)]:\s+(.*)$""")

    // 格式 1：2026-05-31T18:14:00Z INFO routing: message
    private val kernelLogPattern1 =
        Pattern.compile("""^([\d\-T:.Z+]+)\s+([A-Z]+)\s+([^:]+):\s+(.*)$""")

    // 格式 2：INFO[0000] routing: message 或 INFO[0000] [12345] routing: message
    private val kernelLogPattern2 =
        Pattern.compile("""^([A-Z]+)\[\d+]\s+(?:\[\d+]\s+)?([^:]+):\s+(.*)$""")

    // 格式 3：+0800 2026-05-31 16:12:12 INFO network: message
    private val kernelLogPattern3 =
        Pattern.compile("""^([+-]\d{4})\s+([\d\-:\s]+)\s+([A-Z]+)\s+([^:]+):\s+(.*)$""")

    // 路由日志：routed connection from X to Y [outbound]
    private val routingPattern =
        Pattern.compile("""routed connection from (\S+) to (\S+) \[(\S+)]""")

    // 出站连接明细（如 outbound connection to 1.1.1.1:443）
    private val outboundConnPattern =
        Pattern.compile("""^outbound\s+(?:packet\s+)?connection\s+to\s+(\S+)$""")

    fun parse(content: String, type: LogType): List<LogItem> =
        content.lineSequence()
            .filter(String::isNotBlank)
            .map { parseLogLine(it, type) }
            .toList()

    private fun parseLogLine(line: String, type: LogType): LogItem {
        val safeLine = if (type == LogType.SERVICE) LogSanitizer.sanitizeLog(line) else line

        return when (type) {
            LogType.SERVICE -> {
                val matcher1 = serviceLogPattern1.matcher(safeLine)
                if (matcher1.matches()) {
                    val timestamp = matcher1.group(1).orEmpty()
                    val levelStr = matcher1.group(2).orEmpty()
                    val message = matcher1.group(3).orEmpty()
                    val level = parseLogLevel(levelStr)
                    LogItem(
                        rawLine = safeLine,
                        timestamp = timestamp,
                        level = level,
                        tag = "System",
                        message = message
                    )
                } else {
                    val matcher2 = serviceLogPattern2.matcher(safeLine)
                    if (matcher2.matches()) {
                        val levelStr = matcher2.group(1).orEmpty()
                        val message = matcher2.group(2).orEmpty()
                        val level = parseLogLevel(levelStr)
                        LogItem(
                            rawLine = safeLine,
                            timestamp = "",
                            level = level,
                            tag = "System",
                            message = message
                        )
                    } else {
                        LogItem(
                            rawLine = safeLine,
                            timestamp = "",
                            level = guessLogLevel(safeLine),
                            tag = "System",
                            message = safeLine
                        )
                    }
                }
            }

            LogType.KERNEL -> {
                val m3 = kernelLogPattern3.matcher(safeLine)
                if (m3.matches()) {
                    val timestamp = formatDateTimeTimestamp(m3.group(2).orEmpty().trim())
                    val levelStr = m3.group(3).orEmpty()
                    val tag = m3.group(4).orEmpty()
                    val message = m3.group(5).orEmpty()
                    val level = parseLogLevel(levelStr)
                    val flow = parseOutboundFlow(message) ?: parseDetailOutboundFlow(message, tag)
                    LogItem(safeLine, timestamp, level, tag, message, flow)
                } else {
                    val m1 = kernelLogPattern1.matcher(safeLine)
                    if (m1.matches()) {
                        val timestamp = formatIsoTimestamp(m1.group(1).orEmpty())
                        val levelStr = m1.group(2).orEmpty()
                        val tag = m1.group(3).orEmpty()
                        val message = m1.group(4).orEmpty()
                        val level = parseLogLevel(levelStr)
                        val flow =
                            parseOutboundFlow(message) ?: parseDetailOutboundFlow(message, tag)
                        LogItem(safeLine, timestamp, level, tag, message, flow)
                    } else {
                        val m2 = kernelLogPattern2.matcher(safeLine)
                        if (m2.matches()) {
                            val levelStr = m2.group(1).orEmpty()
                            val tag = m2.group(2).orEmpty()
                            val message = m2.group(3).orEmpty()
                            val level = parseLogLevel(levelStr)
                            val flow =
                                parseOutboundFlow(message) ?: parseDetailOutboundFlow(message, tag)
                            LogItem(safeLine, "", level, tag, message, flow)
                        } else {
                            LogItem(safeLine, "", LogLevel.UNKNOWN, "Kernel", safeLine)
                        }
                    }
                }
            }
        }
    }

    private fun parseLogLevel(levelStr: String): LogLevel {
        return when (levelStr.uppercase()) {
            "DEBUG" -> LogLevel.DEBUG
            "INFO" -> LogLevel.INFO
            "WARN", "WARNING" -> LogLevel.WARN
            "ERROR", "FATAL" -> LogLevel.ERROR
            else -> LogLevel.UNKNOWN
        }
    }

    private fun guessLogLevel(line: String): LogLevel {
        val lower = line.lowercase()
        return when {
            lower.contains("error") || lower.contains("fail") || lower.contains("失败") || lower.contains(
                "err"
            ) -> LogLevel.ERROR

            lower.contains("warn") || lower.contains("warning") || lower.contains("警告") -> LogLevel.WARN
            else -> LogLevel.INFO
        }
    }

    private fun parseOutboundFlow(message: String): OutboundFlow? {
        val matcher = routingPattern.matcher(message)
        if (matcher.find()) {
            return OutboundFlow(
                source = matcher.group(1).orEmpty(),
                target = matcher.group(2).orEmpty(),
                outbound = matcher.group(3).orEmpty()
            )
        }
        return null
    }

    private fun parseDetailOutboundFlow(message: String, tag: String): OutboundFlow? {
        val matcher = outboundConnPattern.matcher(message)
        if (matcher.find()) {
            val target = matcher.group(1).orEmpty()
            val outbound = parseOutboundFromTag(tag)
            return OutboundFlow(
                source = "Local",
                target = target,
                outbound = outbound
            )
        }
        return null
    }

    private fun parseOutboundFromTag(tag: String): String {
        return try {
            val idx = tag.indexOf("outbound/")
            if (idx != -1) {
                val sub = tag.substring(idx + "outbound/".length)
                val start = sub.indexOf('[')
                val end = sub.indexOf(']')
                if (start != -1 && end != -1 && end > start) {
                    sub.substring(start + 1, end)
                } else {
                    sub
                }
            } else {
                tag
            }
        } catch (_: Exception) {
            tag
        }
    }

    private fun formatIsoTimestamp(isoStr: String): String {
        // 例：2026-05-31T18:14:00Z -> 05-31 18:14:00
        return try {
            if (isoStr.length >= 19) {
                val datePart = isoStr.substring(5, 10) // 05-31
                val timePart = isoStr.substring(11, 19) // 18:14:00
                "$datePart $timePart"
            } else {
                isoStr
            }
        } catch (_: Exception) {
            isoStr
        }
    }

    private fun formatDateTimeTimestamp(dtStr: String): String {
        // 例：2026-05-31 16:12:12 -> 05-31 16:12:12
        return try {
            if (dtStr.length >= 19) {
                dtStr.substring(5, 19)
            } else {
                dtStr
            }
        } catch (_: Exception) {
            dtStr
        }
    }
}
