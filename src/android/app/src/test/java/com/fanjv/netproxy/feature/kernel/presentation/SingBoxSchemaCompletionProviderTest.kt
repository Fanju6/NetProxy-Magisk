package com.fanjv.netproxy.feature.kernel.presentation

import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import top.yukonga.scripta.editor.completion.CompletionRequest
import top.yukonga.scripta.editor.text.TextPosition

class SingBoxSchemaCompletionProviderTest {
    private val provider = SingBoxSchemaCompletionProvider(TEST_SCHEMA)

    @Test
    fun `property completion follows type discriminator`() = runBlocking {
        val document = """
            {
              "inbounds": [
                {
                  "type": "ebpf",
                  ""
                }
              ]
            }
        """.trimIndent()
        val caretOffset = document.indexOf("\"\"", document.indexOf("ebpf")) + 1

        val result = provider.complete(document.requestAt(caretOffset))
        val labels = requireNotNull(result).items.map { it.label }

        assertTrue("dns_mode" in labels)
        assertTrue("cgroup_enabled" in labels)
        assertTrue("shared_network" in labels)
        assertFalse("listen_port" in labels)
        assertFalse("type" in labels)
    }

    @Test
    fun `value completion returns enum values inside string`() = runBlocking {
        val document = """
            {
              "inbounds": [
                {
                  "type": "ebpf",
                  "dns_mode": ""
                }
              ]
            }
        """.trimIndent()
        val caretOffset = document.indexOf("\"\"", document.indexOf("dns_mode")) + 1

        val result = provider.complete(document.requestAt(caretOffset))

        val items = requireNotNull(result).items
        assertEquals(listOf("hijack", "off"), items.map { it.label })
        assertEquals("hijack", items.first().insertText)
    }

    @Test
    fun `explicit property completion inserts complete key prefix`() = runBlocking {
        val document = "{\n  \n}"
        val caretOffset = document.indexOf("  ") + 2

        val result = provider.complete(document.requestAt(caretOffset, explicit = true))
        val dns = requireNotNull(result).items.first { it.label == "dns" }

        assertEquals("\"dns\": ", dns.insertText)
    }

    @Test
    fun `array value completion offers discriminator snippets`() = runBlocking {
        val markedDocument = """
            {
              "inbounds": [
                <caret>
              ]
            }
        """.trimIndent()
        val caretOffset = markedDocument.indexOf("<caret>")
        val document = markedDocument.replace("<caret>", "")

        val result = provider.complete(document.requestAt(caretOffset, explicit = true))
        val ebpf = requireNotNull(result).items.first { it.label == "ebpf type" }

        assertEquals("{\"type\":\"ebpf\"}", ebpf.insertText)
        assertEquals("配置片段", ebpf.detail)
    }

    @Test
    fun `context help exposes json path and chinese field meaning`() {
        val document = """
            {
              "inbounds": [
                {
                  "type": "ebpf"
                }
              ]
            }
        """.trimIndent()
        val caretOffset = document.indexOf("ebpf") + 2

        val help = provider.contextHelp(document, document.positionAt(caretOffset))

        assertEquals("$.inbounds[0].type", requireNotNull(help).path)
        assertEquals("type", help.field)
        assertTrue(help.documentation.orEmpty().contains("配置对象的类型"))
    }

    private fun String.requestAt(offset: Int, explicit: Boolean = false): CompletionRequest =
        CompletionRequest(
            text = this,
            caret = positionAt(offset),
            explicit = explicit,
        )

    private fun String.positionAt(offset: Int): TextPosition {
        val prefix = substring(0, offset)
        val line = prefix.count { it == '\n' }
        val lineStart = prefix.lastIndexOf('\n').let { if (it < 0) 0 else it + 1 }
        return TextPosition(line, offset - lineStart)
    }

    private companion object {
        val TEST_SCHEMA = """
            {
              "type": "object",
              "properties": {
                "dns": { "type": "object" },
                "inbounds": {
                  "type": "array",
                  "items": { "${'$'}ref": "#/${'$'}defs/Inbound" }
                }
              },
              "${'$'}defs": {
                "Inbound": {
                  "oneOf": [
                    {
                      "type": "object",
                      "properties": {
                        "type": { "const": "ebpf" },
                        "tag": { "type": "string" },
                        "cgroup_enabled": { "type": "boolean" },
                        "dns_mode": {
                          "type": "string",
                          "enum": ["hijack", "off"],
                          "default": "hijack"
                        },
                        "shared_network": { "type": "object" }
                      },
                      "required": ["type"]
                    },
                    {
                      "type": "object",
                      "properties": {
                        "type": { "const": "socks" },
                        "tag": { "type": "string" },
                        "listen_port": { "type": "integer" }
                      },
                      "required": ["type"]
                    }
                  ]
                }
              }
            }
        """.trimIndent()
    }
}


