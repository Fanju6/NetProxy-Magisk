package com.fanjv.netproxy.feature.logs.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** 分享或展示诊断信息前移除日志与配置中的敏感值。 */
internal object LogSanitizer {
    private const val REDACTED_URL = "[订阅链接已隐藏]"
    private const val REDACTED_CONFIG_VALUE = "[配置值已隐藏]"
    private val httpUrlPattern = Regex("""(?i)\bhttps?://[^\s\p{Cc}<>"']+""")
    private val json = Json { prettyPrint = true }
    private val sensitiveConfigKeys = setOf(
        "WIFI_SSID_LIST",
        "PROXY_APPS_LIST",
        "BYPASS_APPS_LIST"
    )
    private val sensitiveJsonKeys = setOf(
        "password",
        "secret",
        "token",
        "access_token",
        "refresh_token",
        "client_secret",
        "api_key",
        "private_key",
        "pre_shared_key",
        "uuid",
        "authorization",
        "proxy_authorization",
        "cookie",
        "set_cookie",
        "hwid"
    )

    fun sanitizeLog(content: String): String =
        httpUrlPattern.replace(content, REDACTED_URL)

    fun sanitizeShellConfig(content: String): String =
        content.split('\n').joinToString("\n") { line ->
            val trimmed = line.trimStart()
            val separator = line.indexOf('=')
            if (trimmed.startsWith('#') || separator <= 0) return@joinToString line

            val key = line.substring(0, separator).trim()
            if (isSensitiveConfigKey(key)) {
                "${line.substring(0, separator + 1)}\"$REDACTED_CONFIG_VALUE\""
            } else {
                line
            }
        }

    fun sanitizeJsonConfig(content: String): String = runCatching {
        val root = json.parseToJsonElement(content)
        json.encodeToString(JsonElement.serializer(), sanitizeJsonElement(root))
    }.getOrElse {
        """{"_error":"配置无法安全解析，已从日志报告中省略"}"""
    }

    private fun isSensitiveConfigKey(key: String): Boolean {
        val upper = key.uppercase()
        return upper in sensitiveConfigKeys ||
                upper.contains("SECRET") ||
                upper.contains("TOKEN") ||
                upper.contains("PASSWORD") ||
                upper.contains("HWID")
    }

    private fun sanitizeJsonElement(element: JsonElement): JsonElement = when (element) {
        is JsonObject -> JsonObject(
            element.mapValues { (key, value) ->
                if (isSensitiveJsonKey(key)) {
                    JsonPrimitive(REDACTED_CONFIG_VALUE)
                } else {
                    sanitizeJsonElement(value)
                }
            }
        )

        is JsonArray -> JsonArray(element.map(::sanitizeJsonElement))
        else -> element
    }

    private fun normalizeJsonKey(key: String): String =
        key.lowercase().replace('-', '_')

    private fun isSensitiveJsonKey(key: String): Boolean {
        val normalized = normalizeJsonKey(key)
        return normalized in sensitiveJsonKeys ||
                normalized.contains("password") ||
                normalized.contains("secret") ||
                normalized.contains("token") ||
                normalized.contains("private_key") ||
                normalized.contains("pre_shared_key") ||
                normalized.contains("authorization") ||
                normalized.contains("cookie") ||
                normalized.contains("hwid")
    }
}
