package com.fanjv.netproxy.core.ui

import kotlinx.coroutines.CancellationException
import org.junit.Assert.assertEquals
import org.junit.Test

class ThrowableExtensionsTest {
    @Test(expected = CancellationException::class)
    fun `cancellation is never converted to a user error`() {
        CancellationException("qz2 was cancelled").userMessage()
    }

    @Test
    fun `regular errors keep their readable message`() {
        assertEquals("读取状态失败", IllegalStateException("读取状态失败").userMessage())
    }
}
