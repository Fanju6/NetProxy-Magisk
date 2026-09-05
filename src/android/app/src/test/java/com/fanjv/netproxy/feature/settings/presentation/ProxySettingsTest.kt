package com.fanjv.netproxy.feature.settings.presentation

import org.junit.Assert.assertEquals
import org.junit.Test

class ProxySettingsTest {
    @Test
    fun dnsModesAreIndependentForLocalAndSharedDataPaths() {
        val settings = ProxySettings(
            localEnabled = true,
            sharedEnabled = true,
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
            localEnabled = true,
            sharedEnabled = true,
            localBypassPrivateAddress = true,
            sharedBypassPrivateAddress = false,
        )

        assertEquals(true, settings.localBypassPrivateAddress)
        assertEquals(false, settings.sharedBypassPrivateAddress)
    }

    @Test
    fun ipv6SwitchesAreIndependentForLocalAndSharedDataPaths() {
        val settings = ProxySettings(
            localEnabled = true,
            sharedEnabled = true,
            localIpv6 = false,
            sharedIpv6 = true,
        )

        assertEquals(false, settings.localIpv6)
        assertEquals(true, settings.sharedIpv6)
    }

    @Test
    fun portBypassIsIndependentForLocalAndSharedDataPaths() {
        val settings = ProxySettings(
            localEnabled = true,
            sharedEnabled = true,
            localBypassPorts = "53,853",
            localBypassPortRanges = "8000:8080",
            sharedBypassPorts = "67,68",
            sharedBypassPortRanges = "10000:10100",
        )

        assertEquals("53,853", settings.localBypassPorts)
        assertEquals("8000:8080", settings.localBypassPortRanges)
        assertEquals("67,68", settings.sharedBypassPorts)
        assertEquals("10000:10100", settings.sharedBypassPortRanges)
    }

    @Test
    fun dataPathSelectionIsDerivedFromEnablement() {
        assertEquals("local", ProxySettings(localEnabled = true, sharedEnabled = false).dataPathSelection)
        assertEquals("shared", ProxySettings(localEnabled = false, sharedEnabled = true).dataPathSelection)
        assertEquals("both", ProxySettings(localEnabled = true, sharedEnabled = true).dataPathSelection)
    }
}
