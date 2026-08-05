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
        assertEquals("", values["EMPTY"])
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
}
