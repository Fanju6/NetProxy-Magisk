package com.fanjv.netproxy.feature.inbound.presentation

import com.fanjv.netproxy.feature.inbound.data.*
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test

class InboundListInputTest {
    @Test fun nativeArraysRoundTripWithoutSplittingOrTrimmingTokens() {
        val native = inboundJson.parseToJsonElement("""{
            "include_interface":["ap,0","usb0"],
            "route_exclude_address_set":["corp,lan"," full width，tag "],
            "multi_queue":true
        }""").jsonObject
        for (field in listOf("include_interface", "route_exclude_address_set")) {
            val original = native.listAt(field)
            val input = InboundListInput(original)
            assertEquals(original, input.withText(input.text).entries())
            assertEquals(native, native.withFields(field to stringArray(input.entries())))
        }
    }

    @Test fun manualInputSplitsOnlyLinesAndPreservesCommasAndWhitespace() {
        val input = InboundListInput(emptyList()).withText("corp,lan\r\n full width，tag \n\ncorp,lan")
        assertEquals(listOf("corp,lan", " full width，tag "), input.entries())
        assertEquals(listOf("ap,0", "usb0"), input.withText("ap,0\nusb0").entries())
    }

    @Test fun candidateTogglesOperateOnWholeListValues() {
        val input = InboundListInput(listOf("manual,tag", "corp,lan"))
        val added = input.withSelection("full，width", true)
        assertEquals(listOf("manual,tag", "corp,lan", "full，width"), added.entries())
        assertEquals(added, added.withSelection("corp,lan", true))
        assertEquals(listOf("manual,tag", "full，width"), added.withSelection("corp,lan", false).entries())
        assertEquals(added, added.withSelection("corp", false))
    }

    @Test fun trailingNewlineSurvivesEditingAndEmptyInputClearsList() {
        val input = InboundListInput(emptyList()).withText("ap0\n")
        assertEquals("ap0\n", input.text)
        assertEquals(listOf("ap0"), input.entries())
        assertEquals(emptyList<String>(), input.withText("").entries())
    }
}
