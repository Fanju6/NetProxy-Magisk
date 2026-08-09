package com.fanjv.netproxy.feature.kernel.presentation

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class SingBoxSchemaValidatorTest {
    private val validator = SingBoxSchemaValidator(TEST_SCHEMA)

    @Test
    fun validDocumentPassesDeclaredSchema() = runBlocking {
        val result = validator.validate(
            """{"${'$'}schema":"$SCHEMA_URI","name":"NetProxy"}""",
        )

        assertEquals(SingBoxSchemaValidationResult.Valid, result)
    }

    @Test
    fun invalidDocumentReturnsLocatedIssue() = runBlocking {
        val result = validator.validate(
            """{"${'$'}schema":"$SCHEMA_URI","name":7}""",
        )

        assertTrue(result is SingBoxSchemaValidationResult.Invalid)
        val issue = (result as SingBoxSchemaValidationResult.Invalid).issues.single()
        assertEquals("/name", issue.instancePath)
        assertEquals(1, issue.line)
        assertTrue((issue.column ?: 0) > 0)
    }

    @Test
    fun documentWithoutSchemaIsStillValidated() = runBlocking {
        val result = validator.validate("""{"name":"NetProxy"}""")

        assertEquals(SingBoxSchemaValidationResult.Valid, result)
    }

    @Test
    fun branchValidationReturnsTheSelectedProtocolFieldError() = runBlocking {
        val branchValidator = SingBoxSchemaValidator(BRANCH_SCHEMA)

        val result = branchValidator.validate(
            """
                {
                  "items": [
                    { "type": "one", "port": 0 }
                  ]
                }
            """.trimIndent(),
        )

        assertTrue(result is SingBoxSchemaValidationResult.Invalid)
        val issue = (result as SingBoxSchemaValidationResult.Invalid).issues.single()
        assertEquals("/items/0/port", issue.instancePath)
        assertTrue(issue.message.contains("不能小于"))
        assertEquals(3, issue.line)
    }

    @Test
    fun durationPatternRequiresUnitsAndAllowsCompoundValues() = runBlocking {
        val durationValidator = SingBoxSchemaValidator(DURATION_SCHEMA)

        assertEquals(
            SingBoxSchemaValidationResult.Valid,
            durationValidator.validate("""{"timeout":"300ms"}"""),
        )
        assertTrue(durationValidator.validate("""{"timeout":"300"}""") is SingBoxSchemaValidationResult.Invalid)
        assertEquals(
            SingBoxSchemaValidationResult.Valid,
            durationValidator.validate("""{"timeout":"1h30m"}"""),
        )
    }

    @Test
    fun bundledSchemaSelectsLogicalDnsRuleByDiscriminator() = runBlocking {
        val schemaFile = sequenceOf(
            File("src/main/assets/sing-box.schema.json"),
            File("app/src/main/assets/sing-box.schema.json"),
        ).first(File::isFile)
        val bundledValidator = SingBoxSchemaValidator(schemaFile.readText())

        assertEquals(
            SingBoxSchemaValidationResult.Valid,
            bundledValidator.validate(
                """
                    {
                      "dns": {
                        "rules": [
                          {
                            "type": "logical",
                            "mode": "and",
                            "rules": [],
                            "action": "route"
                          }
                        ]
                      }
                    }
                """.trimIndent(),
            ),
        )
        assertTrue(
            bundledValidator.validate(
                """
                    {
                      "dns": {
                        "rules": [
                          {
                            "type": "unsupported",
                            "mode": "and",
                            "rules": [],
                            "action": "route"
                          }
                        ]
                      }
                    }
                """.trimIndent(),
            ) is SingBoxSchemaValidationResult.Invalid,
        )
    }

    @Test
    fun allOfTracksEvaluatedPropertiesBeforeRejectingUnknownField() = runBlocking {
        val branchValidator = SingBoxSchemaValidator(BRANCH_SCHEMA)

        val result = branchValidator.validate(
            """
                {
                  "logical": {
                    "type": "logical",
                    "mode": "and",
                    "unexpected": true
                  }
                }
            """.trimIndent(),
        )

        assertTrue(result is SingBoxSchemaValidationResult.Invalid)
        val issues = (result as SingBoxSchemaValidationResult.Invalid).issues
        assertEquals(listOf("/logical/unexpected"), issues.map { it.instancePath })
        assertTrue(issues.single().message.contains("不允许字段"))
    }

    @Test
    fun malformedJsonReturnsTheParserLocation() = runBlocking {
        val result = validator.validate(
            """
                {
                  "name":
                }
            """.trimIndent(),
        )

        assertTrue(result is SingBoxSchemaValidationResult.Invalid)
        val issue = (result as SingBoxSchemaValidationResult.Invalid).issues.single()
        assertEquals("", issue.instancePath)
        assertTrue((issue.line ?: 0) > 0)
        assertTrue((issue.column ?: 0) > 0)
        assertTrue(issue.message.startsWith("JSON 语法错误："))
    }

    @Test
    fun additionalPropertiesRejectsUnknownFields() = runBlocking {
        val result = validator.validate(
            """{"${'$'}schema":"$SCHEMA_URI","name":"NetProxy","unknown":true}""",
        )

        assertTrue(result is SingBoxSchemaValidationResult.Invalid)
        val issue = (result as SingBoxSchemaValidationResult.Invalid).issues.single()
        assertEquals("/unknown", issue.instancePath)
        assertTrue(issue.message.contains("不允许字段"))
    }

    @Test
    fun anyOfAndPropertyNamesValidateTheirSelectedBranch() = runBlocking {
        val branchValidator = SingBoxSchemaValidator(BRANCH_SCHEMA)

        val result = branchValidator.validate(
            """
                {
                  "labels": [1],
                  "interfaces": { "rmnet0": "192.0.2.1" }
                }
            """.trimIndent(),
        )

        assertTrue(result is SingBoxSchemaValidationResult.Invalid)
        val issues = (result as SingBoxSchemaValidationResult.Invalid).issues
        assertEquals(
            setOf("/labels/0", "/interfaces/rmnet0"),
            issues.map(SingBoxSchemaIssue::instancePath).toSet(),
        )
    }

    @Test
    fun bundledSingBoxSchemaValidatesConfigFragment() = runBlocking {
        val schemaFile = sequenceOf(
            File("src/main/assets/sing-box.schema.json"),
            File("app/src/main/assets/sing-box.schema.json"),
        ).first(File::isFile)
        val bundledValidator = SingBoxSchemaValidator(schemaFile.readText())
        val result = bundledValidator.validate(
            """
                {
                  "${'$'}schema": "https://sing-box.sagernet.org/schema.json",
                  "log": { "level": "info" },
                  "inbounds": [
                    {
                      "type": "mixed",
                      "tag": "mixed-in",
                      "listen": "::",
                      "listen_port": 7080
                    }
                  ]
                }
            """.trimIndent(),
        )

        assertEquals(SingBoxSchemaValidationResult.Valid, result)
    }

    @Test
    fun bundledSchemaSupportsRef1ndExtensions() = runBlocking {
        val schemaFile = sequenceOf(
            File("src/main/assets/sing-box.schema.json"),
            File("app/src/main/assets/sing-box.schema.json"),
        ).first(File::isFile)
        val bundledValidator = SingBoxSchemaValidator(schemaFile.readText())
        val result = bundledValidator.validate(
            """
                {
                  "${'$'}schema": "$REF1ND_SCHEMA_URI",
                  "dns": {
                    "servers": [
                      {
                        "type": "group",
                        "tag": "dns-proxy",
                        "servers": ["cloudflare", "google"]
                      }
                    ]
                  },
                  "route": {
                    "rules": [
                      { "action": "sniff-override-destination" }
                    ]
                  },
                  "providers": []
                }
            """.trimIndent(),
        )

        assertEquals(SingBoxSchemaValidationResult.Valid, result)
    }

    @Test
    fun bundledSchemaOnlyUsesSupportedValidationKeywords() {
        val schemaFile = sequenceOf(
            File("src/main/assets/sing-box.schema.json"),
            File("app/src/main/assets/sing-box.schema.json"),
        ).first(File::isFile)
        val schema = singBoxSchemaJson.parseToJsonElement(schemaFile.readText()).jsonObject

        assertTrue((schema.validationKeywords() - SUPPORTED_SCHEMA_KEYWORDS).isEmpty())
    }

    @Test
    fun bundledSchemaValidatesAllStaticModuleDocuments() = runBlocking {
        val schemaFile = sequenceOf(
            File("src/main/assets/sing-box.schema.json"),
            File("app/src/main/assets/sing-box.schema.json"),
        ).first(File::isFile)
        val configDirectory = sequenceOf(
            File("../module/config/singbox/confdir"),
            File("../../module/config/singbox/confdir"),
            File("src/module/config/singbox/confdir"),
        ).first(File::isDirectory)
        val validator = SingBoxSchemaValidator(schemaFile.readText())

        val failures = configDirectory.listFiles { file -> file.extension == "json" }
            .orEmpty()
            .sortedBy(File::getName)
            .mapNotNull { file ->
                (validator.validate(file.readText()) as? SingBoxSchemaValidationResult.Invalid)
                    ?.let { file.name to it.issues.joinToString { issue -> issue.message } }
            }

        assertTrue("静态配置校验失败：$failures", failures.isEmpty())
    }

    private companion object {
        const val SCHEMA_URI = "https://schemas.example.com/netproxy.json"
        const val REF1ND_SCHEMA_URI =
            "https://raw.githubusercontent.com/reF1nd/sing-box/reF1nd-testing/docs/schema.json"
        val TEST_SCHEMA = """
            {
              "${'$'}schema": "https://json-schema.org/draft/2020-12/schema",
              "type": "object",
              "properties": {
                "${'$'}schema": { "type": "string" },
                "name": { "type": "string" }
              },
              "required": ["name"],
              "additionalProperties": false
            }
        """.trimIndent()

        val DURATION_SCHEMA = """
            {
              "type": "object",
              "properties": {
                "timeout": {
                  "${'$'}ref": "#/${'$'}defs/Duration"
                }
              },
              "required": ["timeout"],
              "additionalProperties": false,
              "${'$'}defs": {
                "Duration": {
                  "type": "string",
                  "pattern": "^[-+]?(((\\d+(\\.\\d*)?|\\.\\d+)(ns|us|µs|μs|ms|s|m|h|d))+|0)$"
                }
              }
            }
        """.trimIndent()

        val BRANCH_SCHEMA = """
            {
              "type": "object",
              "properties": {
                "items": {
                  "type": "array",
                  "items": { "${'$'}ref": "#/${'$'}defs/Item" }
                },
                "logical": { "${'$'}ref": "#/${'$'}defs/Logical" },
                "labels": {
                  "anyOf": [
                    { "type": "string" },
                    { "type": "array", "items": { "type": "string" } }
                  ]
                },
                "interfaces": {
                  "type": "object",
                  "propertyNames": { "enum": ["wlan0"] },
                  "additionalProperties": { "type": "string" }
                }
              },
              "additionalProperties": false,
              "${'$'}defs": {
                "Item": {
                  "oneOf": [
                    {
                      "type": "object",
                      "properties": {
                        "type": { "const": "one" },
                        "port": { "type": "integer", "minimum": 1, "maximum": 65535 }
                      },
                      "required": ["type", "port"],
                      "additionalProperties": false
                    },
                    {
                      "type": "object",
                      "properties": {
                        "type": { "const": "two" },
                        "name": { "type": "string" }
                      },
                      "required": ["type"],
                      "additionalProperties": false
                    }
                  ]
                },
                "Logical": {
                  "type": "object",
                  "unevaluatedProperties": false,
                  "allOf": [
                    {
                      "properties": {
                        "type": { "const": "logical" }
                      },
                      "required": ["type"]
                    },
                    {
                      "properties": {
                        "mode": { "enum": ["and", "or"] }
                      },
                      "required": ["mode"]
                    }
                  ]
                }
              }
            }
        """.trimIndent()

    }
}

private val SUPPORTED_SCHEMA_KEYWORDS = setOf(
    "\$ref",
    "type",
    "properties",
    "required",
    "additionalProperties",
    "unevaluatedProperties",
    "items",
    "enum",
    "const",
    "oneOf",
    "anyOf",
    "allOf",
    "minimum",
    "maximum",
    "pattern",
    "propertyNames",
)

private fun JsonObject.validationKeywords(): Set<String> {
    val keywords = linkedSetOf<String>()

    fun JsonElement.collect() {
        val schema = this as? JsonObject ?: return
        keywords += schema.keys.intersect(SUPPORTED_SCHEMA_KEYWORDS + UNSUPPORTED_SCHEMA_KEYWORDS)
        schema["properties"].asSchemaObject()?.values?.forEach { it.collect() }
        schema["\$defs"].asSchemaObject()?.values?.forEach { it.collect() }
        schema["items"]?.collect()
        schema["propertyNames"]?.collect()
        schema["additionalProperties"]?.collect()
        schema["unevaluatedProperties"]?.collect()
        listOf("oneOf", "anyOf", "allOf").forEach { key ->
            (schema[key] as? JsonArray).orEmpty().forEach { it.collect() }
        }
    }

    collect()
    return keywords
}

private fun JsonElement?.asSchemaObject(): JsonObject? = this as? JsonObject

private val UNSUPPORTED_SCHEMA_KEYWORDS = setOf(
    "not",
    "if",
    "then",
    "else",
    "dependentRequired",
    "dependentSchemas",
    "patternProperties",
    "minProperties",
    "maxProperties",
    "prefixItems",
    "contains",
    "minContains",
    "maxContains",
    "minItems",
    "maxItems",
    "uniqueItems",
    "minLength",
    "maxLength",
    "multipleOf",
    "exclusiveMinimum",
    "exclusiveMaximum",
)
