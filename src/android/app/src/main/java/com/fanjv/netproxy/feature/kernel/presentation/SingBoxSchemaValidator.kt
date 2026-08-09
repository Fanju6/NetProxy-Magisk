package com.fanjv.netproxy.feature.kernel.presentation

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.math.BigDecimal

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

/** 使用内置 reF1nd sing-box Schema 校验配置，不访问网络。 */
class SingBoxSchemaValidator private constructor(
    private val schemaProvider: () -> String,
) {
    constructor(context: Context) : this({
        context.assets.open(SCHEMA_ASSET).bufferedReader().use { it.readText() }
    })

    internal constructor(schemaContent: String) : this({ schemaContent })

    private val schemaRoot by lazy(LazyThreadSafetyMode.SYNCHRONIZED) {
        runCatching { singBoxSchemaJson.parseToJsonElement(schemaProvider()).jsonObject }
    }

    suspend fun validate(rawJson: String): SingBoxSchemaValidationResult =
        withContext(Dispatchers.Default) {
            val document = runCatching { singBoxSchemaJson.parseToJsonElement(rawJson) }
                .getOrElse { error ->
                    return@withContext SingBoxSchemaValidationResult.Invalid(
                        listOf(jsonSyntaxIssue(rawJson, error)),
                    )
                }
            val schema = schemaRoot.getOrElse { error ->
                return@withContext SingBoxSchemaValidationResult.Unavailable(
                    error.message ?: error.javaClass.simpleName,
                )
            }
            val sourceIndex = buildJsonSourceIndex(rawJson)
            val issues = SingBoxSchemaEngine(schema, sourceIndex)
                .validate(document)
                .compactSchemaIssues()

            if (issues.isEmpty()) {
                SingBoxSchemaValidationResult.Valid
            } else {
                SingBoxSchemaValidationResult.Invalid(issues)
            }
        }

    private companion object {
        const val SCHEMA_ASSET = "sing-box.schema.json"
    }
}

/**
 * 只实现 reF1nd sing-box Schema 当前实际使用的关键字。
 *
 * 这不是通用 JSON Schema 引擎：不支持远程引用，也不尝试覆盖未被内置
 * Schema 使用的草案关键字。Schema 更新时应由测试确认新增关键字。
 */
private class SingBoxSchemaEngine(
    root: JsonObject,
    private val sourceIndex: Map<String, JsonSourcePosition>,
) {
    private val references = SingBoxSchemaReferenceResolver(root)

    fun validate(document: JsonElement): List<SingBoxSchemaIssue> =
        validateValue(document, rootSchema, "").issues

    private val rootSchema = root

    private fun validateValue(
        value: JsonElement,
        schema: JsonObject,
        path: String,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ): ValidationOutcome {
        if (depth > MAX_SCHEMA_DEPTH) {
            return ValidationOutcome(
                issues = listOf(issue(path, "配置层级过深，无法继续校验")),
            )
        }

        val issues = mutableListOf<SingBoxSchemaIssue>()
        val evaluatedProperties = linkedSetOf<String>()
        references.referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            val referenced = validateValue(value, target, path, visitedRefs + ref, depth + 1)
            issues += referenced.issues
            evaluatedProperties += referenced.evaluatedProperties
        }

        validateType(value, schema, path)?.let(issues::add)
        validateConstAndEnum(value, schema, path)?.let(issues::add)
        validateNumber(value, schema, path, issues)
        validateString(value, schema, path, issues)

        schema["allOf"].asSchemaArray().forEach { child ->
            val result = validateValue(value, child, path, visitedRefs, depth + 1)
            issues += result.issues
            evaluatedProperties += result.evaluatedProperties
        }

        validateCombination(value, schema, path, "oneOf", visitedRefs, depth)?.let { result ->
            issues += result.issues
            evaluatedProperties += result.evaluatedProperties
        }
        validateCombination(value, schema, path, "anyOf", visitedRefs, depth)?.let { result ->
            issues += result.issues
            evaluatedProperties += result.evaluatedProperties
        }

        when (value) {
            is JsonObject -> {
                val objectResult = validateObject(value, schema, path, visitedRefs, depth)
                issues += objectResult.issues
                evaluatedProperties += objectResult.evaluatedProperties
                validateUnevaluatedProperties(value, schema, path, evaluatedProperties, issues)
            }

            is JsonArray -> validateArray(value, schema, path, visitedRefs, depth, issues)
            else -> Unit
        }
        return ValidationOutcome(issues, evaluatedProperties)
    }

    private fun validateType(
        value: JsonElement,
        schema: JsonObject,
        path: String,
    ): SingBoxSchemaIssue? {
        val allowedTypes = schema["type"].schemaTypes()
        if (allowedTypes.isEmpty() || valueMatchesTypes(value, allowedTypes)) return null
        return issue(
            path,
            "应为${allowedTypes.joinToString("或") { localizedType(it) }}，实际为${
                localizedType(
                    jsonType(value)
                )
            }",
        )
    }

    private fun validateConstAndEnum(
        value: JsonElement,
        schema: JsonObject,
        path: String,
    ): SingBoxSchemaIssue? {
        schema["const"]?.let { expected ->
            if (value != expected) return issue(path, "值必须为 ${displayJsonValue(expected)}")
        }
        val allowedValues = schema["enum"] as? JsonArray
        if (allowedValues != null && value !in allowedValues) {
            return issue(
                path,
                "值不在允许范围内：${allowedValues.joinToString { displayJsonValue(it) }}"
            )
        }
        return null
    }

    private fun validateNumber(
        value: JsonElement,
        schema: JsonObject,
        path: String,
        issues: MutableList<SingBoxSchemaIssue>,
    ) {
        if (jsonType(value) !in NUMBER_TYPES) return
        val number = value.jsonPrimitive.content.toBigDecimalOrNull() ?: return
        schema["minimum"].asDecimal()?.let { minimum ->
            if (number < minimum) issues += issue(path, "数值不能小于 $minimum")
        }
        schema["maximum"].asDecimal()?.let { maximum ->
            if (number > maximum) issues += issue(path, "数值不能大于 $maximum")
        }
    }

    private fun validateString(
        value: JsonElement,
        schema: JsonObject,
        path: String,
        issues: MutableList<SingBoxSchemaIssue>,
    ) {
        if (jsonType(value) != "string") return
        schema["pattern"]?.jsonPrimitive?.contentOrNull?.let { pattern ->
            val matches =
                runCatching { Regex(pattern).containsMatchIn(value.jsonPrimitive.content) }
                    .getOrDefault(true)
            if (!matches) issues += issue(path, "字符串格式不符合要求")
        }
    }

    private fun validateCombination(
        value: JsonElement,
        schema: JsonObject,
        path: String,
        keyword: String,
        visitedRefs: Set<String>,
        depth: Int,
    ): ValidationOutcome? {
        val branches = schema[keyword].asSchemaArray()
        if (branches.isEmpty()) return null

        val candidates = matchingBranches(value, branches, visitedRefs, depth)
        val results = candidates.map { branch ->
            validateValue(value, branch, path, visitedRefs, depth + 1)
        }
        val validResults = results.filter { it.issues.isEmpty() }
        val hasDiscriminator = candidates.size < branches.size

        return when (keyword) {
            "oneOf" -> when {
                validResults.size == 1 -> validResults.single()
                hasDiscriminator && results.size == 1 -> results.single()
                else -> ValidationOutcome(
                    issues = listOf(
                        issue(
                            path,
                            if (validResults.isEmpty()) "不匹配任何可用配置类型" else "同时匹配多个配置类型",
                        ),
                    ),
                )
            }

            else -> when {
                validResults.isNotEmpty() -> validResults.first()
                hasDiscriminator && results.size == 1 -> results.single()
                else -> ValidationOutcome(
                    issues = listOf(issue(path, "不符合任何允许的配置格式")),
                )
            }
        }
    }

    private fun matchingBranches(
        value: JsonElement,
        branches: List<JsonObject>,
        visitedRefs: Set<String>,
        depth: Int,
    ): List<JsonObject> {
        val matches = branches.filter { branch ->
            branchMatchesValue(branch, value, visitedRefs, depth + 1)
        }
        return matches.ifEmpty { branches }
    }

    private fun branchMatchesValue(
        schema: JsonObject,
        value: JsonElement,
        visitedRefs: Set<String>,
        depth: Int,
    ): Boolean {
        if (depth > MAX_SCHEMA_DEPTH) return false
        if (value is JsonObject) {
            val constraints = discriminatorConstraints(schema, visitedRefs, depth + 1)
            if (constraints.isNotEmpty()) {
                return constraints.all { (name, allowed) ->
                    value[name]?.let { it in allowed } == true
                }
            }
        }
        val allowedTypes = schema["type"].schemaTypes()
        return allowedTypes.isEmpty() || valueMatchesTypes(value, allowedTypes)
    }

    private fun discriminatorConstraints(
        schema: JsonObject,
        visitedRefs: Set<String>,
        depth: Int,
    ): Map<String, Set<JsonElement>> {
        if (depth > MAX_SCHEMA_DEPTH) return emptyMap()
        val result = linkedMapOf<String, Set<JsonElement>>()
        schema["properties"].asSchemaObject()?.let { properties ->
            DISCRIMINATOR_FIELDS.forEach { field ->
                val property = properties[field].asSchemaObject() ?: return@forEach
                val values = property["const"]?.let(::setOf)
                    ?: (property["enum"] as? JsonArray)?.toSet()?.takeIf { it.isNotEmpty() }
                if (values != null) result[field] = values
            }
        }
        references.referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            mergeDiscriminatorConstraints(
                result,
                discriminatorConstraints(target, visitedRefs + ref, depth + 1),
            )
        }
        schema["allOf"].asSchemaArray().forEach { child ->
            mergeDiscriminatorConstraints(
                result,
                discriminatorConstraints(child, visitedRefs, depth + 1),
            )
        }
        return result
    }

    private fun mergeDiscriminatorConstraints(
        target: MutableMap<String, Set<JsonElement>>,
        source: Map<String, Set<JsonElement>>,
    ) {
        source.forEach { (field, values) ->
            target[field] = target[field]?.intersect(values) ?: values
        }
    }

    private fun validateObject(
        value: JsonObject,
        schema: JsonObject,
        path: String,
        visitedRefs: Set<String>,
        depth: Int,
    ): ValidationOutcome {
        val issues = mutableListOf<SingBoxSchemaIssue>()
        val evaluatedProperties = linkedSetOf<String>()
        val properties = schema["properties"].asSchemaObject().orEmpty()
        val required = schema["required"].asStringSet()

        required.filterNot(value::containsKey).forEach { name ->
            issues += issue(path, "缺少必填字段 \"$name\"")
        }
        properties.forEach { (name, propertySchema) ->
            val propertyValue = value[name] ?: return@forEach
            val child = propertySchema.asSchemaObject() ?: return@forEach
            evaluatedProperties += name
            val result = validateValue(
                propertyValue,
                child,
                "$path/${escapeJsonPointerSegment(name)}",
                visitedRefs,
                depth + 1,
            )
            issues += result.issues
        }

        schema["propertyNames"].asSchemaObject()?.let { nameSchema ->
            value.keys.forEach { name ->
                val result = validateValue(
                    JsonPrimitive(name),
                    nameSchema,
                    "$path/${escapeJsonPointerSegment(name)}",
                    visitedRefs,
                    depth + 1,
                )
                issues += result.issues
            }
        }

        val additionalProperties = schema["additionalProperties"]
        value.forEach { (name, propertyValue) ->
            if (name in properties) return@forEach
            when {
                (additionalProperties as? JsonPrimitive)?.booleanOrNull == false -> {
                    issues += issue(
                        "$path/${escapeJsonPointerSegment(name)}",
                        "不允许字段 \"$name\""
                    )
                }

                additionalProperties is JsonObject -> {
                    evaluatedProperties += name
                    val result = validateValue(
                        propertyValue,
                        additionalProperties,
                        "$path/${escapeJsonPointerSegment(name)}",
                        visitedRefs,
                        depth + 1,
                    )
                    issues += result.issues
                }
            }
        }
        return ValidationOutcome(issues, evaluatedProperties)
    }

    private fun validateUnevaluatedProperties(
        value: JsonObject,
        schema: JsonObject,
        path: String,
        evaluatedProperties: MutableSet<String>,
        issues: MutableList<SingBoxSchemaIssue>,
    ) {
        if ((schema["unevaluatedProperties"] as? JsonPrimitive)?.booleanOrNull != false) return
        value.keys.filterNot(evaluatedProperties::contains).forEach { name ->
            issues += issue("$path/${escapeJsonPointerSegment(name)}", "不允许字段 \"$name\"")
        }
    }

    private fun validateArray(
        value: JsonArray,
        schema: JsonObject,
        path: String,
        visitedRefs: Set<String>,
        depth: Int,
        issues: MutableList<SingBoxSchemaIssue>,
    ) {
        val itemSchema = schema["items"].asSchemaObject() ?: return
        value.forEachIndexed { index, item ->
            val result = validateValue(item, itemSchema, "$path/$index", visitedRefs, depth + 1)
            issues += result.issues
        }
    }

    private fun issue(path: String, message: String): SingBoxSchemaIssue {
        val location = sourceIndex[path] ?: sourceIndex[path.substringBeforeLast('/', "")]
        return SingBoxSchemaIssue(
            message = message,
            instancePath = path,
            line = location?.line,
            column = location?.column,
        )
    }

    private companion object {
        const val MAX_SCHEMA_DEPTH = 64
        val DISCRIMINATOR_FIELDS = setOf("type", "action")
        val NUMBER_TYPES = setOf("number", "integer")
    }
}

private data class ValidationOutcome(
    val issues: List<SingBoxSchemaIssue> = emptyList(),
    val evaluatedProperties: Set<String> = emptySet(),
)

private fun jsonSyntaxIssue(rawJson: String, error: Throwable): SingBoxSchemaIssue {
    val offset = JSON_OFFSET_REGEX.find(error.message.orEmpty())
        ?.groupValues
        ?.getOrNull(1)
        ?.toIntOrNull()
    val location = offset?.let { sourcePositionAt(rawJson, it) }
    return SingBoxSchemaIssue(
        message = "JSON 语法错误：${error.message ?: "无法解析配置"}",
        instancePath = "",
        line = location?.line,
        column = location?.column,
    )
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

private fun String.isBranchSummary(): Boolean =
    "配置类型" in this || "允许的配置格式" in this

private fun JsonElement?.asSchemaObject(): JsonObject? = this as? JsonObject

private fun JsonElement?.asSchemaArray(): List<JsonObject> =
    (this as? JsonArray).orEmpty().mapNotNull { it as? JsonObject }

private fun JsonElement?.asStringSet(): Set<String> =
    (this as? JsonArray).orEmpty().mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.toSet()

private fun JsonElement?.schemaTypes(): Set<String> = when (this) {
    is JsonPrimitive -> contentOrNull?.let(::setOf).orEmpty()
    is JsonArray -> mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.toSet()
    else -> emptySet()
}

private fun JsonElement?.asDecimal(): BigDecimal? =
    (this as? JsonPrimitive)?.contentOrNull?.toBigDecimalOrNull()

private fun jsonType(value: JsonElement): String = when (value) {
    is JsonObject -> "object"
    is JsonArray -> "array"
    JsonNull -> "null"
    is JsonPrimitive -> when {
        value.isString -> "string"
        value.booleanOrNull != null -> "boolean"
        value.content.toBigDecimalOrNull()?.stripTrailingZeros()?.scale()
            ?.let { it <= 0 } == true -> "integer"

        else -> "number"
    }
}

private fun valueMatchesTypes(value: JsonElement, allowedTypes: Set<String>): Boolean {
    val actualType = jsonType(value)
    return actualType in allowedTypes || actualType == "integer" && "number" in allowedTypes
}

private fun localizedType(type: String): String = when (type) {
    "string" -> "字符串"
    "integer" -> "整数"
    "number" -> "数字"
    "boolean" -> "布尔值"
    "object" -> "对象"
    "array" -> "数组"
    "null" -> "空值"
    else -> type
}

private fun displayJsonValue(value: JsonElement): String =
    if (value is JsonPrimitive && value.isString) "\"${value.content}\"" else value.toString()

private fun escapeJsonPointerSegment(value: String): String =
    value.replace("~", "~0").replace("/", "~1")

private const val MAX_ISSUES_PER_PATH = 3
private const val MAX_VISIBLE_SCHEMA_ISSUES = 20
private val JSON_OFFSET_REGEX = Regex("\\boffset\\s+(\\d+)")
