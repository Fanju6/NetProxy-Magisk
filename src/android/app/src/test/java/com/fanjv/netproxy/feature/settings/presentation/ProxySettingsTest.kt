package com.fanjv.netproxy.feature.settings.presentation

import org.junit.Assert.assertEquals
import org.junit.Test

class ProxySettingsTest {
    @Test
    fun dnsModesAreIndependentForLocalAndSharedDataPaths() {
        val settings = ProxySettings(
            mode = "hybrid",
            network = "tcp",
            localDnsMode = "respect_policy",
            sharedDnsMode = "off",
        )

        assertEquals("tcp", settings.network)
        assertEquals("respect_policy", settings.localDnsMode)
        assertEquals("off", settings.sharedDnsMode)
    }

    @Test
    fun privateAddressBypassIsIndependentForLocalAndSharedDataPaths() {
        val settings = ProxySettings(
            mode = "hybrid",
            localBypassPrivateAddress = true,
            sharedBypassPrivateAddress = false,
        )

        assertEquals(true, settings.localBypassPrivateAddress)
        assertEquals(false, settings.sharedBypassPrivateAddress)
    }
}
