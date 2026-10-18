package com.fanjv.netproxy.bridge

import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test

class PublicCatalogTest {
    private val secret = "private-auth-marker"
    private fun group(type: String = "subscription") = Json.parseToJsonElement("""{
      "id":"default","type":"$type","provider_tag":"Group","name":"Group",
      "active":true,"runtime_tag":"$secret",
      "node_count":1,"revision":2,"auto_update":true,"update_interval":3600,"update_via_proxy":"auto",
      "profile_title":"Group","last_attempt_at":"","last_success_at":"","next_update_at":"","updated_at":"",
      "usage":{"total":100,"download":2,"hwid":"$secret"},"progress":null,
      "url":"https://example.invalid/?token=$secret","custom_headers":{"Authorization":"$secret"},
      "profile_web_page_url":"https://example.invalid/?token=$secret","hwid":"$secret","last_error":"$secret"
    }""")

    @Test fun listsDetailsAndEventSummariesDropPrivateFieldsBeforeIPC() {
        for (type in listOf("local", "subscription")) {
            val item = group(type)
            val list = JsonArray(listOf(item))
            assertFalse(PublicCatalog.read("/subscriptions", list).toString().contains(secret))
            assertFalse(PublicCatalog.event("subscriptions", list).toString().contains(secret))
            val detail = buildJsonObject {
                put("group", item)
                put("nodes", buildJsonArray { add(buildJsonObject { put("tag", "one"); put("protocol", "vless"); put("password", secret) }) })
                put("url", secret)
            }
            val result = PublicCatalog.read("/subscriptions/default", detail).jsonObject
            assertFalse(result.toString().contains(secret))
            assertEquals(type, result.getValue("group").jsonObject.getValue("type").jsonPrimitive.content)
            val snapshot = buildJsonObject {
                put("groups", JsonArray(listOf(detail)))
                put("selection", secret); put("runtime_groups", secret)
            }
            val nodes = PublicCatalog.read("/nodes/snapshot", snapshot).jsonObject
            assertEquals(nodes, PublicCatalog.read("/catalog", snapshot))
            assertEquals(setOf("groups"), nodes.keys)
            assertFalse(nodes.toString().contains(secret))
            assertFalse(result.getValue("group").jsonObject.containsKey("active"))
            assertFalse(result.getValue("group").jsonObject.containsKey("runtime_tag"))
            assertEquals("Group", result.getValue("group").jsonObject.getValue("provider_tag").jsonPrimitive.content)
        }
        assertEquals(JsonArray(emptyList()), PublicCatalog.read("/subscriptions", JsonArray(emptyList())))
        val empty = buildJsonObject { put("groups", JsonArray(emptyList())) }
        assertEquals(empty, PublicCatalog.read("/catalog", empty))
    }

    @Test fun historyProgressAndErrorsNeverForwardRawExceptionText() {
        val history = Json.parseToJsonElement("""[{"at":"2026-10-17T00:00:00Z","ok":false,"code":"subscription.convert_failed","message":"$secret","diagnostics":[{"password":"$secret"}]}]""")
        val safe = PublicCatalog.read("/subscriptions/default/history", history).toString()
        assertFalse(safe.contains(secret))
        assertTrue(safe.contains("subscription.convert_failed"))
        for (stage in listOf("download", "convert", "validate", "apply")) {
            val progress = Json.parseToJsonElement("""{"id":"default","progress":{"schema":1,"group_id":"default","stage":"$stage","updated_at":"2026-10-17T00:00:00Z","message":"$secret","url":"$secret"}}""")
            assertFalse(PublicCatalog.read("/subscriptions/default/progress", progress).toString().contains(secret))
        }
    }

    @Test fun privateMetadataWrongGroupAndNestedSensitiveValuesFailClosed() {
        for (data in listOf(Json.parseToJsonElement("""{"id":"default","type":"subscription","url":"$secret"}"""), JsonObject(group().jsonObject - "provider_tag"))) {
            assertTrue(runCatching { PublicCatalog.read("/subscriptions", JsonArray(listOf(data))) }.exceptionOrNull() is BridgeFailure)
        }
        val nested = JsonObject(group().jsonObject + ("name" to buildJsonObject { put("secret", secret) }))
        assertTrue(runCatching { PublicCatalog.read("/subscriptions", JsonArray(listOf(nested))) }.exceptionOrNull() is BridgeFailure)
        val mismatched = Json.parseToJsonElement("""{"id":"other","progress":null}""")
        assertTrue(runCatching { PublicCatalog.read("/subscriptions/default/progress", mismatched) }.exceptionOrNull() is BridgeFailure)
    }
}
