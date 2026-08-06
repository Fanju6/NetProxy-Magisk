package com.fanjv.netproxy.feature.settings.singbox

import android.content.Context
import com.networknt.schema.InputFormat
import com.networknt.schema.SchemaRegistry
import com.networknt.schema.SpecificationVersion
import com.networknt.schema.utils.JsonNodes
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

data class SingBoxSchemaIssue(
    val message: String,
    val instancePath: String,
    val line: Int?,
    val column: Int?,
)

sealed interface SingBoxSchemaValidationResult {
    data object Valid : SingBoxSchemaValidationResult

    data class Invalid(
        val issues: List<SingBoxSchemaIssue>,
    ) : SingBoxSchemaValidationResult

    data class Unavailable(
        val reason: String,
    ) : SingBoxSchemaValidationResult
}

/** 使用应用内置的 sing-box Schema 校验配置，不访问网络。 */
class SingBoxSchemaValidator private constructor(
    private val schemaProvider: () -> String,
) {
    constructor(context: Context) : this({
        context.assets.open(SCHEMA_ASSET).bufferedReader().use { it.readText() }
    })

    internal constructor(schemaContent: String) : this({ schemaContent })

    private val schema by lazy(LazyThreadSafetyMode.SYNCHRONIZED) {
        SchemaRegistry.withDefaultDialect(
            SpecificationVersion.DRAFT_2020_12,
        ) { builder ->
            builder.nodeReader { nodeReader -> nodeReader.locationAware() }
        }.getSchema(schemaProvider(), InputFormat.JSON)
    }

    suspend fun validate(rawJson: String): SingBoxSchemaValidationResult =
        withContext(Dispatchers.IO) {
            runCatching {
                val issues = schema.validate(rawJson, InputFormat.JSON)
                    .map { error ->
                        val location = error.instanceNode?.let(JsonNodes::tokenStreamLocationOf)
                        SingBoxSchemaIssue(
                            message = error.message,
                            instancePath = error.instanceLocation.toString(),
                            line = location?.lineNr?.takeIf { it > 0 },
                            column = location?.columnNr?.takeIf { it > 0 },
                        )
                    }
                    .sortedWith(
                        compareBy(
                            { it.line ?: Int.MAX_VALUE },
                            { it.column ?: Int.MAX_VALUE })
                    )
                    .compactSchemaIssues()

                if (issues.isEmpty()) {
                    SingBoxSchemaValidationResult.Valid
                } else {
                    SingBoxSchemaValidationResult.Invalid(issues)
                }
            }.getOrElse { error ->
                SingBoxSchemaValidationResult.Unavailable(
                    reason = error.message ?: error.javaClass.simpleName,
                )
            }
        }

    private companion object {
        const val SCHEMA_ASSET = "sing-box.schema.json"
    }
}

private fun List<SingBoxSchemaIssue>.compactSchemaIssues(): List<SingBoxSchemaIssue> {
    val distinctIssues = distinctBy { Triple(it.instancePath, it.line, it.message) }
    return distinctIssues
        .groupBy(SingBoxSchemaIssue::instancePath)
        .values
        .flatMap { issues ->
            if (issues.size <= MAX_ISSUES_PER_PATH) {
                issues
            } else {
                issues.filterNot { it.message.isBranchSummary() }
                    .ifEmpty { issues.take(1) }
                    .take(MAX_ISSUES_PER_PATH)
            }
        }
        .sortedWith(compareBy({ it.line ?: Int.MAX_VALUE }, { it.column ?: Int.MAX_VALUE }))
        .take(MAX_VISIBLE_SCHEMA_ISSUES)
}

private fun String.isBranchSummary(): Boolean {
    val value = lowercase()
    return "oneof" in value || "anyof" in value || "exactly one" in value ||
            "一个且仅一个" in this || "子架构" in this
}

private const val MAX_ISSUES_PER_PATH = 3
private const val MAX_VISIBLE_SCHEMA_ISSUES = 20
