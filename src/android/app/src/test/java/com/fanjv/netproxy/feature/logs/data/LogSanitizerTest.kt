package com.fanjv.netproxy.feature.logs.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LogSanitizerTest {
    @Test
    fun redactsSubscriptionUrlsAndCredentials() {
        val source = """
            update https://user:password@example.com/sub?token=secret
            retry HTTP://example.net/api/subscription?id=123
        """.trimIndent()

        val sanitized = LogSanitizer.sanitizeLog(source)

        assertEquals(
            """
                update [订阅链接已隐藏]
                retry [订阅链接已隐藏]
            """.trimIndent(),
            sanitized
        )
        assertFalse(sanitized.contains("secret"))
        assertFalse(sanitized.contains("example.com"))
    }

    @Test
    fun preservesNonUrlDiagnosticText() {
        val source = "订阅更新失败: timeout"

        assertEquals(source, LogSanitizer.sanitizeLog(source))
    }

    @Test
    fun redactsSensitiveShellConfigValues() {
        val source = """
            AUTO_START=1
            WIFI_SSID_LIST="Home WiFi"
            PROXY_APPS_LIST="com.example.private"
            API_TOKEN=secret-token
        """.trimIndent()

        val sanitized = LogSanitizer.sanitizeShellConfig(source)

        assertTrue(sanitized.contains("AUTO_START=1"))
        assertFalse(sanitized.contains("Home WiFi"))
        assertFalse(sanitized.contains("com.example.private"))
        assertFalse(sanitized.contains("secret-token"))
    }

    @Test
    fun redactsSensitiveJsonFieldsRecursively() {
        val source = """
            {
              "secret": "controller-secret",
              "outbounds": [
                {
                  "server": "proxy.example.com",
                  "uuid": "private-uuid",
                  "headers": {
                    "Authorization": "Bearer private-token"
                  }
                }
              ]
            }
        """.trimIndent()

        val sanitized = LogSanitizer.sanitizeJsonConfig(source)

        assertTrue(sanitized.contains("proxy.example.com"))
        assertFalse(sanitized.contains("controller-secret"))
        assertFalse(sanitized.contains("private-uuid"))
        assertFalse(sanitized.contains("private-token"))
    }

    @Test
    fun omitsMalformedJsonInsteadOfLeakingSecrets() {
        val sanitized = LogSanitizer.sanitizeJsonConfig("""{"secret":"private-value"""")

        assertTrue(sanitized.contains("配置无法安全解析"))
        assertFalse(sanitized.contains("private-value"))
    }
}
