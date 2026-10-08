package com.fanjv.netproxy.feature.kernel.presentation

import com.fanjv.netproxy.R
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test
import top.yukonga.scripta.editor.completion.CompletionRequest
import top.yukonga.scripta.editor.text.TextPosition

class SingBoxJsonProcessingTest {
    @Test fun validationAndCompletionShareOneImmutableSchemaAndResolver() = runBlocking {
        val reads = AtomicInteger()
        val caller = Thread.currentThread()
        val schema = SingBoxEditorSchema {
            assertNotSame(caller, Thread.currentThread())
            reads.incrementAndGet()
            singBoxSchemaJson.parseToJsonElement(
                """{"type":"object","properties":{"enabled":{"type":"boolean"}}}"""
            ).jsonObject
        }
        val text = localizedSchemaText("")
        val validator = SingBoxSchemaValidator(schema, text)
        val provider = SingBoxSchemaCompletionProvider(schema, text)
        val validation = async { validator.validate("""{"enabled":true}""") }
        val completion = async { provider.complete(CompletionRequest(
            text = "{\"en", caret = TextPosition(0, 4), explicit = true,
        )) }
        assertEquals(SingBoxSchemaValidationResult.Valid, validation.await())
        assertEquals("enabled", completion.await()?.items?.single()?.label)
        assertEquals(1, reads.get())
        assertSame(schema, schema.forDocument("singbox/dns"))
        assertSame(schema.references, schema.references)
        assertEquals(SingBoxSchemaValidationResult.Valid, validator.validate("{}"))
        assertEquals(1, reads.get())
    }

    @Test fun formattingPreservesValuesAndStrictSyntaxValidationWorksWithoutSchema() = runBlocking {
        val raw = """{"a/b":"中😀","nested":[true,null,{"value":7}],"escaped":"line\nnext"}"""
        val parsed = parseEditorJson(raw)
        val formatted = formatEditorJson(parsed)
        assertTrue(formatted.contains('\n'))
        assertEquals(parsed, parseEditorJson(formatted))
        val text = localizedSchemaText("")
        assertEquals(SingBoxSchemaValidationResult.Valid, validateEditorJson(formatted, null, text))
        for (invalid in listOf("{\"value\":}", "{unquoted:true}", "[]")) {
            assertTrue(validateEditorJson(invalid, null, text) is SingBoxSchemaValidationResult.Invalid)
        }
    }

    @Test fun longSchemaValidationStopsCooperativelyAndPropagatesCancellation() = runBlocking {
        val job = Job()
        val rendered = AtomicInteger()
        val schema = testEditorSchema("""{"type":"array","items":{"type":"integer"}}""")
        val validator = SingBoxSchemaValidator(schema, SchemaText { id, _ ->
            if (id == R.string.schema_type_mismatch && rendered.incrementAndGet() == 5) job.cancel()
            "type mismatch"
        })
        try {
            withContext(job) { validator.validate(List(20_000) { "\"bad\"" }.joinToString(",", "[", "]")) }
            fail("Expected cancellation")
        } catch (_: CancellationException) {
            assertEquals(5, rendered.get())
        }
    }

    @Test fun schemaLoadingCancellationIsNotConvertedToUnavailable() = runBlocking {
        val cancellation = CancellationException("Schema loading cancelled")
        val validator = SingBoxSchemaValidator(SingBoxEditorSchema { throw cancellation }, localizedSchemaText(""))
        try {
            validator.validate("{}")
            fail("Expected cancellation")
        } catch (error: CancellationException) {
            assertEquals(cancellation.message, error.message)
        }
    }

    @Test fun cancelledSyntaxAndFormattingDoNotReturnResults() = runBlocking {
        val parsed = parseEditorJson("{}")
        for (operation in listOf<suspend () -> Any>(
            { validateEditorJson("{}", null, localizedSchemaText("")) },
            { parseEditorJson("{}") },
            { formatEditorJson(parsed) },
        )) {
            val job = Job().apply { cancel() }
            try {
                withContext(job) { operation() }
                fail("Expected cancellation")
            } catch (_: CancellationException) {
                assertTrue(job.isCancelled)
            }
        }
    }

    @Test fun schemaErrorsRetainExactMultilineValueLocation() = runBlocking {
        val schema = testEditorSchema("""{"type":"object","properties":{"port":{"type":"integer"}}}""")
        val result = validateEditorJson("{\r\n  \"port\": \"bad\"\r\n}",
            SingBoxSchemaValidator(schema, localizedSchemaText("")), localizedSchemaText(""))
            as SingBoxSchemaValidationResult.Invalid
        assertEquals("/port", result.issues.single().instancePath)
        assertEquals(2, result.issues.single().line)
        assertEquals(11, result.issues.single().column)
    }
}
