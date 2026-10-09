package com.fanjv.netproxy.feature.kernel.presentation

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test
import top.yukonga.scripta.editor.completion.CompletionRequest
import top.yukonga.scripta.editor.text.TextPosition
import java.io.File

class InboundEditorSchemaTest {
    private val source = File("src/main/assets/sing-box.schema.json").readText()
    private val ebpf = """{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"dns_mode":"respect_policy"},"shared":{"enabled":false}}"""
    private val tun = """{"type":"tun","tag":"netproxy-in","address":["172.19.0.1/30","fdfe:dcba:9876::1/126"],"auto_route":true,"auto_redirect":true,"dns_mode":"hijack"}"""
    private val app = """{"enabled":true,"mode":"blacklist","proxy_apps":["0:com.example.client","10:com.example.client"],"bypass_apps":[]}"""

    private val schema = testEditorSchema(source)
    private fun validator(target: String) = SingBoxSchemaValidator(schema.forDocument(target), localizedSchemaText(""))

    @Test fun fullWrapperRequiresExactlyFiveFields() = runBlocking {
        val value = """{"backend":"ebpf","root_policy":"default","app":$app,"ebpf":$ebpf,"tun":$tun}"""
        assertEquals(SingBoxSchemaValidationResult.Valid, validator("inbound").validate(value))
        val parsed = singBoxSchemaJson.parseToJsonElement(value).jsonObject
        for (field in parsed.keys) {
            assertTrue(validator("inbound").validate(JsonObject(parsed - field).toString()) is SingBoxSchemaValidationResult.Invalid)
        }
        assertTrue(validator("inbound").validate(JsonObject(parsed + ("extra" to JsonPrimitive(1))).toString()) is SingBoxSchemaValidationResult.Invalid)
        assertTrue(validator("inbound").validate(value.replace("\"blacklist\"", "\"invalid\"")) is SingBoxSchemaValidationResult.Invalid)
        assertTrue(validator("inbound").validate(value.replace("\"default\"", "\"invalid\"")) is SingBoxSchemaValidationResult.Invalid)
        assertTrue(validator("inbound").validate(value.replace("0:com.example.client", "com.example.client")) is SingBoxSchemaValidationResult.Invalid)
    }

    @Test fun partitionsUseNativeSchemaRatherThanSingBoxRoot() = runBlocking {
        assertEquals(SingBoxSchemaValidationResult.Valid, validator("inbound/ebpf").validate("""{"ebpf":$ebpf}"""))
        assertEquals(SingBoxSchemaValidationResult.Valid, validator("inbound/tun").validate("""{"tun":$tun}"""))
        assertEquals(SingBoxSchemaValidationResult.Valid, validator("inbound/backend").validate("""{"backend":"tun"}"""))
        assertTrue(validator("inbound/backend").validate("""{"backend":"direct"}""") is SingBoxSchemaValidationResult.Invalid)
        for (policy in listOf("default", "include", "exclude")) {
            assertEquals(SingBoxSchemaValidationResult.Valid, validator("inbound/root_policy").validate("""{"root_policy":"$policy"}"""))
        }
        assertTrue(validator("inbound/root_policy").validate("""{"root_policy":"auto"}""") is SingBoxSchemaValidationResult.Invalid)
        assertTrue(validator("inbound/tun").validate(tun) is SingBoxSchemaValidationResult.Invalid)
        assertTrue(validator("inbound/ebpf").validate("""{"ebpf":{"type":"tun","tag":"netproxy-in"}}""") is SingBoxSchemaValidationResult.Invalid)
        assertTrue(validator("inbound/tun").validate("""{"tun":$tun}""".replace("netproxy-in", "wrong-tag")) is SingBoxSchemaValidationResult.Invalid)
        assertTrue(validator("inbound/tun").validate("""{"tun":$tun}""".replace("\"auto_redirect\":true", "\"auto_redirect\":false")) is SingBoxSchemaValidationResult.Invalid)
    }

    @Test fun originalDefinitionsAreReferencedWithoutNativeFieldCopies() {
        val original = singBoxSchemaJson.parseToJsonElement(source).jsonObject
        val wrapped = editorSchema(original, "inbound")
        assertEquals(original["\$defs"], wrapped["\$defs"])
        for (type in listOf("tun", "ebpf")) {
            val schema = wrapped.getValue("properties").jsonObject.getValue(type).jsonObject
            val reference = (schema.getValue("allOf") as JsonArray).first().jsonObject
            assertTrue(reference.getValue("\$ref").toString().contains("#/\$defs/Inbound/oneOf/"))
            assertNotNull(SingBoxSchemaReferenceResolver(wrapped).referencedSchema(reference, emptySet()))
        }
        assertSame(original, editorSchema(original, "runtime/inbound.json"))
        assertSame(original, editorSchema(original, "runtime/providers.json"))
        assertSame(original, editorSchema(original, "runtime/outbounds.json"))
        assertSame(original, editorSchema(original, "singbox/config.json"))
    }

    @Test fun nativeNumericAndUnknownFieldsAreCheckedByLockedSchema() = runBlocking {
        val value = """{"tun":$tun}"""
        val parsed = singBoxSchemaJson.parseToJsonElement(value).jsonObject.getValue("tun").jsonObject
        assertEquals(SingBoxSchemaValidationResult.Valid, validator("inbound/tun").validate(
            JsonObject(mapOf("tun" to JsonObject(parsed + ("multi_queue" to JsonPrimitive(true))))).toString()
        ))
        for ((field, badValue) in listOf(
            "mtu" to JsonPrimitive(-1), "mtu" to JsonPrimitive("1500"),
            "unknown_native_field" to JsonPrimitive(true), "dns_mode" to JsonPrimitive("off")
        )) {
            assertTrue(validator("inbound/tun").validate(
                JsonObject(mapOf("tun" to JsonObject(parsed + (field to badValue)))).toString()
            ) is SingBoxSchemaValidationResult.Invalid)
        }
    }

    @Test fun completionFollowsNativeReferenceIntoTunAndEbpfLocal() = runBlocking {
        val provider = SingBoxSchemaCompletionProvider(schema.forDocument("inbound"), localizedSchemaText(""))
        for ((text, expected) in listOf(
            "{\"tun\":{\"rou" to "route_exclude_address",
            "{\"ebpf\":{\"local\":{\"dns" to "dns_mode",
            "{\"app\":{\"proxy" to "proxy_apps"
        )) {
            val result = provider.complete(CompletionRequest(text = text, caret = TextPosition(0, text.length), explicit = true))
            assertTrue("$text should complete $expected", result?.items?.any { it.label == expected } == true)
        }
    }
}
