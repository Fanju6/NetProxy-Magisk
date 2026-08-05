package com.fanjv.netproxy.feature.settings.singbox

import java.io.File
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

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
    }
}
