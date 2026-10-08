package com.fanjv.netproxy.feature.kernel.presentation

import com.fanjv.netproxy.R
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import java.util.Locale
import javax.xml.parsers.DocumentBuilderFactory
import top.yukonga.scripta.editor.completion.CompletionRequest
import top.yukonga.scripta.editor.text.TextPosition

class LocalizationResourcesTest {
    @Test
    fun translationsCoverEveryDefaultStringAndPreserveFormatArguments() {
        val english = resourceStrings("")
        val formatArgument = Regex("%[0-9]+\\$[ds]|%%")
        val chinese = Regex("[\\u4e00-\\u9fff]")
        assertFalse(english.values.any { chinese.containsMatchIn(it) })
        for (language in listOf("zh", "ru")) {
            val translated = resourceStrings(language)
            assertEquals(language, english.keys, translated.keys)
            for ((name, value) in english) {
                assertTrue("$language/$name", translated.getValue(name).isNotBlank())
                assertEquals(
                    "$language/$name",
                    formatArgument.findAll(value).map { it.value }.sorted().toList(),
                    formatArgument.findAll(translated.getValue(name)).map { it.value }.sorted().toList(),
                )
            }
            if (language == "ru") assertFalse(translated.values.any { chinese.containsMatchIn(it) })
        }
        assertEquals("unqualifiedResLocale=en", File("src/main/res/resources.properties").readText().trim())
        assertEquals("Dashboard", english.getValue("dashboard"))
        assertEquals("仪表盘", resourceStrings("zh").getValue("dashboard"))
        assertEquals("Обзор", resourceStrings("ru").getValue("dashboard"))
    }

    @Test
    fun validationSemanticsAndPositionsDoNotDependOnTranslatedMessages() = runBlocking {
        val schema = """{"type":"object","properties":{"port":{"type":"integer","minimum":1}},"additionalProperties":false}"""
        val expected = listOf("Value must not be less than 1", "数值不能小于 1", "Значение не должно быть меньше 1")
        for ((index, language) in listOf("", "zh", "ru").withIndex()) {
            val validator = SingBoxSchemaValidator(testEditorSchema(schema), localizedSchemaText(language))
            assertEquals(SingBoxSchemaValidationResult.Valid, validator.validate("""{"port":443}"""))
            val result = validator.validate("""{"port":0}""") as SingBoxSchemaValidationResult.Invalid
            assertEquals(expected[index], result.issues.single().message)
            assertEquals("/port", result.issues.single().instancePath)
            assertEquals(1, result.issues.single().line)
            val invalid = validator.validate("""{"port":"wrong"}""") as SingBoxSchemaValidationResult.Invalid
            assertTrue(invalid.issues.single().message.contains(localizedSchemaText(language)(R.string.schema_type_integer)))
        }
    }

    @Test
    fun branchSummariesDoNotHideSpecificErrorsInAnyLanguage() = runBlocking {
        val schema = """{
            "type":"object",
            "required":["first","second","third"],
            "oneOf":[{"required":["value"]},{"required":["other"]}]
        }"""
        for (language in listOf("", "zh", "ru")) {
            val text = localizedSchemaText(language)
            val validator = SingBoxSchemaValidator(testEditorSchema(schema), text)
            val result = validator.validate("{}") as SingBoxSchemaValidationResult.Invalid
            assertEquals(
                listOf("first", "second", "third").map { text(R.string.schema_missing_field, it) },
                result.issues.map { it.message },
            )
            assertFalse(result.issues.any { it.branchSummary })
        }
    }

    @Test
    fun booleanCompletionUsesSchemaTypesNotTranslatedTypeNames() = runBlocking {
        val schema = """{"type":"object","properties":{"enabled":{"type":"boolean"}}}"""
        val document = """{"enabled": }"""
        for (language in listOf("", "zh", "ru")) {
            val text = localizedSchemaText(language)
            val provider = SingBoxSchemaCompletionProvider(testEditorSchema(schema), text)
            val result = requireNotNull(provider.complete(CompletionRequest(
                text = document,
                caret = TextPosition(0, document.indexOf('}')),
                explicit = true,
            )))
            assertEquals(setOf("false", "true"), result.items.map { it.label }.toSet())
            assertTrue(result.items.all { it.detail == text(R.string.schema_type_boolean) })
            val help = provider.contextHelp(document, TextPosition(0, document.indexOf('}')))
            assertTrue(requireNotNull(help).documentation.orEmpty().contains(text(R.string.schema_doc_enabled)))
        }
    }
}

internal fun localizedSchemaText(language: String): SchemaText {
    val strings = resourceStrings(language).mapKeys { (name, _) -> R.string::class.java.getField(name).getInt(null) }
    return SchemaText { id, args -> String.format(Locale.ROOT, strings.getValue(id), *args) }
}

private fun resourceStrings(language: String): Map<String, String> {
    val directory = if (language.isEmpty()) "values" else "values-$language"
    val factory = DocumentBuilderFactory.newInstance().apply {
        setFeature("http://apache.org/xml/features/disallow-doctype-decl", true)
    }
    val nodes = factory.newDocumentBuilder().parse(File("src/main/res/$directory/strings.xml"))
        .getElementsByTagName("string")
    val result = linkedMapOf<String, String>()
    for (index in 0 until nodes.length) {
        val element = nodes.item(index) as Element
        val name = element.getAttribute("name")
        val value = element.textContent.removeSurrounding("\"").replace("\\\"", "\"").replace("\\'", "'")
        assertFalse("Duplicate string: $directory/$name", result.containsKey(name))
        result[name] = value
    }
    return result
}
