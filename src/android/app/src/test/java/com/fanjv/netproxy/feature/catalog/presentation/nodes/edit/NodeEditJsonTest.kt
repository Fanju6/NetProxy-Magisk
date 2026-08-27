package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test

class NodeEditJsonTest {
    @Test
    fun `single sing-box Listable value is accepted`() {
        val tls = Json.parseToJsonElement("""{"alpn":"h2"}""") as JsonObject

        assertEquals(listOf("h2"), tls["alpn"].listableStrings())
    }

    @Test
    fun `multiple sing-box Listable values preserve order`() {
        val values = JsonArray(listOf(JsonPrimitive("h2"), JsonPrimitive("http/1.1")))

        assertEquals(listOf("h2", "http/1.1"), values.listableStrings())
    }

    @Test
    fun `missing or invalid Listable values are ignored`() {
        assertEquals(emptyList<String>(), null.listableStrings())
        assertEquals(emptyList<String>(), JsonObject(emptyMap()).listableStrings())
    }
}
