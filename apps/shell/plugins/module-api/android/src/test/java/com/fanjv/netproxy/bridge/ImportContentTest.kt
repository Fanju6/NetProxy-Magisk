package com.fanjv.netproxy.bridge

import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayInputStream

class ImportContentTest {
    private fun rejected(code: String, action: () -> Unit) {
        try { action(); fail("expected rejection") } catch (error: BridgeFailure) { assertEquals(code, error.code) }
    }

    @Test fun utf8AndBomAreReadWithoutChangingNativeJsonOrCredentials() {
        val original = "{\"unknown\":{\"中文\":1e20},\"url\":\"https://example.invalid/sub?token=fixture\"}"
        val result = readImportContent("config.json", ByteArrayInputStream(("\uFEFF" + original).toByteArray()))
        assertEquals(original, result.content)
        assertEquals("config.json", result.name)
        assertTrue(result.id.isNotBlank())
    }

    @Test fun actualContentNotMimeDeterminesTextValidityAndSize() {
        rejected("api.import_invalid") { readImportContent("valid.json", ByteArrayInputStream(byteArrayOf(0xc3.toByte(), 0x28))) }
        rejected("api.import_invalid") { importContent("valid.json", "\u0000{}") }
        rejected("api.import_invalid") { importContent("valid.json", " \n\t") }
        rejected("api.body_too_large") { readImportContent("tiny.txt", ByteArrayInputStream(ByteArray(IMPORT_LIMIT + 1) { 65 })) }
        rejected("api.body_too_large") { importContent("tiny.txt", "中".repeat(IMPORT_LIMIT / 3 + 1)) }
        assertEquals(IMPORT_LIMIT, importContent("valid", "x".repeat(IMPORT_LIMIT)).content.length)
    }

    @Test fun displayNameIsBoundedAndCannotBecomeAPath() {
        val result = importContent("../../a\\b\n" + "x".repeat(200), "{}")
        assertEquals(160, result.name.length)
        assertFalse(result.name.contains('/'))
        assertFalse(result.name.contains('\\'))
        assertFalse(result.name.contains('\n'))
    }

    @Test fun inboxConsumesEachShareOnceWithoutWritingOrReplayingOnRecreation() {
        val inbox = ImportInbox()
        val first = importContent("one", "ss://fixture")
        val second = importContent("two", "https://example.invalid/")
        inbox.offer(Result.success(first)); inbox.offer(Result.success(second))
        assertSame(first, inbox.take()); assertNull(inbox.take())
        assertSame(first, inbox.take(1))
        inbox.complete("stale-id"); assertNull(inbox.take(1))
        inbox.complete(first.id)
        assertSame(second, inbox.take(1)); inbox.complete(second.id); assertNull(inbox.take(1))
        inbox.offer(Result.success(first)); inbox.clear(); assertNull(inbox.take())
    }

    @Test fun inboxHasBoundedBackpressureAndErrorsDoNotContainInput() {
        val inbox = ImportInbox()
        repeat(8) { inbox.offer(Result.success(importContent("", "fixture-$it"))) }
        repeat(4) {
            val value = inbox.take()!!
            assertEquals("fixture-$it", value.content)
            inbox.complete(value.id)
        }
        rejected("api.busy") { inbox.take() }
        assertNull(inbox.take())
        inbox.offer(Result.failure(BridgeFailure("api.import_unreadable")))
        rejected("api.import_unreadable") { inbox.take() }
        assertNull(inbox.take())
    }
}
