package com.fanjv.netproxy.core.command

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class ShellConfigFileTest {
    @Test
    fun parsesSupportedAssignmentsWithoutComments() {
        val values = ShellConfigFile.parse(
            """
            # 模块配置
            AUTO_START=1
            ACTIVE_GROUP_ID="default"
            EMPTY=''
            """.trimIndent()
        )

        assertEquals("1", values["AUTO_START"])
        assertEquals("default", values["ACTIVE_GROUP_ID"])
        assertEquals("''", values["EMPTY"])
    }

    @Test
    fun updatesExistingValueAndQuotesWhitespace() {
        val original = "AUTO_START=1\nACTIVE_GROUP_ID=default\n"

        assertEquals(
            "AUTO_START=0\nACTIVE_GROUP_ID=default\n",
            ShellConfigFile.updateValue(original, "AUTO_START", "0")
        )
        assertEquals(
            "AUTO_START=1\nACTIVE_GROUP_ID=\"local nodes\"\n",
            ShellConfigFile.updateValue(original, "ACTIVE_GROUP_ID", "local nodes")
        )
    }

    @Test
    fun rejectsUnsafeKeysAndMultilineValues() {
        assertThrows(IllegalArgumentException::class.java) {
            ShellConfigFile.updateValue("", "../AUTO_START", "1")
        }
        assertThrows(IllegalArgumentException::class.java) {
            ShellConfigFile.updateValue("", "AUTO_START", "1\nDANGEROUS=1")
        }
    }

    @Test fun replacesSpacedKeysInsteadOfAddingDuplicates() {
        val updated = ShellConfigFile.updateValue("  AUTO_START = 1\n# AUTO_START=1\n", "AUTO_START", "0")
        assertEquals("AUTO_START=0\n# AUTO_START=1\n", updated)
        assertEquals("0", ShellConfigFile.parse(updated)["AUTO_START"])
    }

    @Test fun quotedValuesRoundTripWithoutGrowingEscapes() {
        for (value in listOf("Cafe\\Guest", "Guest \"Wi-Fi\"", "中文", "", "literal\\n", "quote\"", "'raw'")) {
            var content = "WIFI=\"\""
            repeat(3) {
                content = ShellConfigFile.updateValue(content, "WIFI", value, forceQuotes = true)
                assertEquals(value, ShellConfigFile.parse(content)["WIFI"])
            }
        }
    }

    @Test fun acceptsGoEscapesAndRejectsMalformedOrForbiddenValues() {
        assertEquals("AAB龙", ShellConfigFile.parse("""VALUE="\x41\101\u0042\U00009f99"""")["VALUE"])
        for (value in listOf("\"\\t\"", "\"\\q\"", "\"\\uD800\"", "\"\\400\"", "\"\\x0\"", "\"unclosed")) {
            assertThrows(IllegalArgumentException::class.java) { ShellConfigFile.parse("VALUE=$value") }
        }
        assertThrows(IllegalStateException::class.java) { ShellConfigFile.parse("VALUE=1\nVALUE = 2") }
    }
}
