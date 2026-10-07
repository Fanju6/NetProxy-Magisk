package com.fanjv.netproxy.feature.inbound.data

import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test

class InboundJsonTest {
    @Test fun tunMissingAndEmptyDnsUseNativeDefaultWithoutWritingIt() {
        for (content in listOf("{}", """{"dns_mode":""}""")) {
            val original = inboundJson.parseToJsonElement(content).jsonObject
            assertEquals("hijack", original.tunDnsMode())
            assertEquals(content, original.toString())
        }
        for (mode in listOf("disabled", "native", "hijack")) {
            val original = inboundJson.parseToJsonElement("""{"dns_mode":"$mode"}""").jsonObject
            assertEquals(mode, original.tunDnsMode())
        }
    }

    @Test fun ebpfOmittedDnsUsesNativeDefaultWithoutWritingIt() {
        val original = inboundJson.parseToJsonElement("""{"enabled":true}""").jsonObject
        assertEquals("respect_policy", original.ebpfDnsMode())
        assertFalse(original.containsKey("dns_mode"))
        assertEquals("off", original.withFields("dns_mode" to JsonPrimitive("off")).ebpfDnsMode())
    }
    @Test fun ebpfEditsPreserveUnknownFieldsAndIndependentPaths() {
        val original = inboundJson.parseToJsonElement("""
            {"type":"ebpf","tag":"netproxy-in","udp_timeout":"5m","tc_priority":2,
             "local":{"enabled":true,"dns_mode":"respect_policy","ipv6":false,"bypass_private_address":true,"bypass_port":[53,853],"cgroup_path":"/sys/fs/cgroup"},
             "shared":{"enabled":false,"dns_mode":"off","ipv6":true,"interface":["ap0"],"bypass_port":[67,68]}}
        """).jsonObject
        val changed = original.withPath(listOf("shared", "dns_mode"), JsonPrimitive("hijack"))
        assertEquals(original["local"], changed["local"])
        assertEquals(original["udp_timeout"], changed["udp_timeout"])
        assertEquals(original["tc_priority"], changed["tc_priority"])
        assertEquals("hijack", changed.objectAt("shared").textAt("dns_mode"))
        assertEquals(listOf("ap0"), changed.objectAt("shared").listAt("interface"))
        assertEquals(listOf("67", "68"), changed.objectAt("shared").listAt("bypass_port"))
    }

    @Test fun listableNativeValuesAndPortNumbersRoundTrip() {
        val objectValue = inboundJson.parseToJsonElement("""{"address":"172.19.0.1/30","include_uid":10001,"bypass_port":53}""").jsonObject
        assertEquals(listOf("172.19.0.1/30"), objectValue.listAt("address"))
        assertEquals(listOf("53"), objectValue.listAt("bypass_port"))
        val changed = objectValue.withFields("bypass_port" to kotlinx.serialization.json.JsonArray(listOf(JsonPrimitive(53), JsonPrimitive(853))))
        assertEquals("""[53,853]""", changed["bypass_port"].toString())
        assertEquals(objectValue["include_uid"], changed["include_uid"])
    }

    @Test fun ipv6UsesOnlyAddressAndHasNoPersistentCache() {
        val original = inboundJson.parseToJsonElement("""{"address":["172.19.0.1/30","fd00:1234::1/126"],"mtu":9000,"dns_address":"172.19.0.2"}""").jsonObject
        val disabled = original.withTunIpv6(false)
        assertEquals(listOf("172.19.0.1/30"), disabled.listAt("address"))
        assertEquals(original.keys, disabled.keys)
        assertEquals(original["dns_address"], disabled["dns_address"])
        val enabled = disabled.withTunIpv6(true)
        assertEquals(listOf("172.19.0.1/30", DEFAULT_TUN_IPV6), enabled.listAt("address"))
        assertEquals(original, original.withTunIpv6(true))
        assertEquals(enabled, enabled.withTunIpv6(true))
    }

    @Test fun interfaceAndMacFiltersAreMutuallyExclusive() {
        for ((include, exclude) in listOf("include_interface" to "exclude_interface", "include_mac_address" to "exclude_mac_address")) {
            val original = inboundJson.parseToJsonElement("""{"$exclude":["old"],"multi_queue":true}""").jsonObject
            val changed = original.withFilter(include, exclude, "include", listOf("new"))
            assertEquals(listOf("new"), changed.listAt(include))
            assertFalse(changed.containsKey(exclude))
            assertEquals(original["multi_queue"], changed["multi_queue"])
            val cleared = changed.withFilter(include, exclude, "all", emptyList())
            assertFalse(cleared.containsKey(include))
            assertFalse(cleared.containsKey(exclude))
        }
    }

    @Test fun editingBypassCidrsPreservesOtherFields() {
        val original = inboundJson.parseToJsonElement("""{"route_exclude_address":["203.0.113.0/24"],"route_address_set":["selected"],"dns_mode":"disabled"}""").jsonObject
        val changed = original.withFields("route_exclude_address" to stringArray(listOf("192.168.0.0/16")))
        assertEquals(original.keys, changed.keys)
        assertEquals(original["route_address_set"], changed["route_address_set"])
        assertEquals(listOf("192.168.0.0/16"), changed.listAt("route_exclude_address"))
    }
}
