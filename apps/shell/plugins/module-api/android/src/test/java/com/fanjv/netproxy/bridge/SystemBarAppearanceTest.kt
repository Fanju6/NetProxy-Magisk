package com.fanjv.netproxy.bridge

import org.junit.Assert.*
import org.junit.Test

class SystemBarAppearanceTest {
    @Test fun onlyResolvedAppearanceCanChangeBothBars() {
        val appearance = SystemBarAppearance()
        val applied = mutableListOf<Boolean>()
        appearance.reapply { applied.add(it) }
        assertTrue(applied.isEmpty())
        for ((theme, light) in listOf("light" to true, "dark" to false, "light" to true)) {
            appearance.update("""{"theme":"$theme"}""")
            appearance.reapply { applied.add(it) }
            assertEquals(light, applied.last())
        }
        assertEquals(listOf(true, false, true), applied)
    }

    @Test fun invalidCommandsCannotOverwriteTheLastConfirmedAppearance() {
        val appearance = SystemBarAppearance()
        appearance.update("""{"theme":"dark"}""")
        for (raw in listOf(
            "{}", "null", "[]", "\"dark\"", "not json",
            """{"theme":"system"}""", """{"theme":"Dark"}""", """{"theme":" dark"}""",
            """{"theme":""}""", """{"theme":null}""", """{"theme":false}""", """{"theme":1}""",
            """{"theme":{"dark":true}}""", """{"request":{"theme":"light"}}""",
            """{"theme":"light","url":"https://localhost"}""",
            """{"theme":"light","path":"/service/start"}""",
            """{"theme":"light","method":"POST"}""", """{"theme":"light","system":true}""",
            """{"theme":"light","unknown":null}""", " ".repeat(129),
        )) {
            val error = runCatching { appearance.update(raw) }.exceptionOrNull()
            assertTrue(raw, error is BridgeFailure)
            assertEquals("api.invalid_request", (error as BridgeFailure).code)
            assertEquals(400, error.status)
            appearance.reapply { assertFalse(it) }
        }
    }

    @Test fun configurationAndReplacementOwnersReceiveTheRetainedWebAppearance() {
        val appearance = SystemBarAppearance()
        appearance.update("""{"theme":"light"}""")
        val oldOwner = mutableListOf<Boolean>()
        val replacementOwner = mutableListOf<Boolean>()
        appearance.reapply { oldOwner.add(it) }
        appearance.reapply { oldOwner.add(it) }
        appearance.reapply { replacementOwner.add(it) }
        assertEquals(listOf(true, true), oldOwner)
        assertEquals(listOf(true), replacementOwner)
        appearance.update("""{"theme":"dark"}""")
        appearance.reapply { replacementOwner.add(it) }
        assertEquals(listOf(true, false), replacementOwner)
        assertEquals(listOf(true, true), oldOwner)
    }
}
