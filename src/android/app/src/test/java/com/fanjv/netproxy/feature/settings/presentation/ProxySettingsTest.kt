package com.fanjv.netproxy.feature.settings.presentation

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ProxySettingsTest {
    @Test
    fun sharedDnsInterceptionRequiresUdpForBothInterceptModes() {
        assertTrue(requiresSharedUdp("shared", "hijack"))
        assertTrue(requiresSharedUdp("hybrid", "respect_bypass"))
        assertFalse(requiresSharedUdp("shared", "off"))
        assertFalse(requiresSharedUdp("local", "hijack"))
    }

    @Test
    fun bypassPrivateAddressReflectsEnabledDataPlanes() {
        assertTrue(
            ProxySettings(
                mode = "local",
                localBypassPrivateAddress = true,
                sharedBypassPrivateAddress = false,
            ).bypassPrivateAddress,
        )
        assertFalse(
            ProxySettings(
                mode = "shared",
                localBypassPrivateAddress = true,
                sharedBypassPrivateAddress = false,
            ).bypassPrivateAddress,
        )
        assertTrue(
            ProxySettings(
                mode = "hybrid",
                localBypassPrivateAddress = true,
                sharedBypassPrivateAddress = false,
            ).bypassPrivateAddress,
        )
        assertFalse(
            ProxySettings(
                mode = "hybrid",
                localBypassPrivateAddress = false,
                sharedBypassPrivateAddress = false,
            ).bypassPrivateAddress,
        )
    }
}
