package com.fanjv.netproxy

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AlphaExpiryTest {
    @Test
    fun `expires exactly at configured deadline`() {
        val deadline = BuildConfig.ALPHA_EXPIRES_AT_MILLIS

        assertFalse(AlphaExpiry.isExpired(deadline - 1))
        assertTrue(AlphaExpiry.isExpired(deadline))
        assertTrue(AlphaExpiry.isExpired(deadline + 1))
    }
}
