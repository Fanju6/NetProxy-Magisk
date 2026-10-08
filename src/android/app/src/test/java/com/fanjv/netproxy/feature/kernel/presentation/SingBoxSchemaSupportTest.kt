package com.fanjv.netproxy.feature.kernel.presentation

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.ensureActive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test

class SingBoxSchemaSupportTest {
    @Test fun positionsMatchUtf16OffsetsWithNestedEscapedKeysAndCrLf() {
        val source = "{\r\n  \"a/b~c\": [\r\n    {\"\\u4e2d\\\"\": \"中😀\\n\"},\r\n    false\r\n  ],\r\n  \"tail\": null\r\n}"
        val positions = buildJsonSourceIndex(source)
        assertEquals(JsonSourcePosition(1, 1), positions[""])
        assertEquals(JsonSourcePosition(2, 12), positions["/a~1b~0c"])
        assertEquals(JsonSourcePosition(3, 5), positions["/a~1b~0c/0"])
        val stringOffset = source.indexOf("\"中😀")
        assertEquals(sourcePositionAt(source, stringOffset), positions["/a~1b~0c/0/中\""])
        assertEquals(JsonSourcePosition(4, 5), positions["/a~1b~0c/1"])
        assertEquals(JsonSourcePosition(6, 11), positions["/tail"])
        assertEquals(JsonSourcePosition(1, 1), sourcePositionAt(source, -1))
        assertEquals(sourcePositionAt(source, source.length), sourcePositionAt(source, Int.MAX_VALUE))
    }

    @Test fun sourceReadsGrowLinearlyAcrossAuditSizes() {
        for (count in listOf(1_000, 2_000, 4_000, 8_000, 16_000)) {
            val source = buildString {
                append("{\n")
                repeat(count) { index ->
                    append("  \"field$index\": $index")
                    if (index < count - 1) append(',')
                    append('\n')
                }
                append('}')
            }
            val counted = CountingText(source)
            val positions = buildJsonSourceIndex(counted)
            assertEquals(count + 1, positions.size)
            assertEquals(sourcePositionAt(source, source.lastIndexOf((count - 1).toString())), positions["/field${count - 1}"])
            assertTrue("$count fields: ${counted.reads} reads", counted.reads <= source.length * 6)
        }
    }

    @Test fun cancellationDuringLargeStringIsNotConvertedToEmptyIndex() {
        val job = Job()
        var checks = 0
        try {
            buildJsonSourceIndex("{\"text\":\"${"x".repeat(100_000)}\"}") {
                if (++checks == 8) job.cancel()
                job.ensureActive()
            }
            fail("Expected cancellation")
        } catch (_: CancellationException) {
            assertEquals(8, checks)
        }
    }

    private class CountingText(private val source: String) : CharSequence {
        var reads = 0
        override val length: Int get() = source.length
        override fun get(index: Int): Char { reads++; return source[index] }
        override fun subSequence(startIndex: Int, endIndex: Int): CharSequence {
            reads += endIndex - startIndex
            return source.subSequence(startIndex, endIndex)
        }
    }
}

internal fun testEditorSchema(source: String): SingBoxEditorSchema =
    SingBoxEditorSchema { singBoxSchemaJson.parseToJsonElement(source).jsonObject }
