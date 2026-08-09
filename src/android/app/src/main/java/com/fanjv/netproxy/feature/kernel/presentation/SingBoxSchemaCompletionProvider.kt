package com.fanjv.netproxy.feature.kernel.presentation

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import top.yukonga.scripta.editor.completion.CompletionItem
import top.yukonga.scripta.editor.completion.CompletionItemKind
import top.yukonga.scripta.editor.completion.CompletionProvider
import top.yukonga.scripta.editor.completion.CompletionRequest
import top.yukonga.scripta.editor.completion.CompletionResult
import top.yukonga.scripta.editor.text.TextPosition
import top.yukonga.scripta.editor.text.TextRange

data class SingBoxSchemaContextHelp(
    val path: String,
    val field: String?,
    val documentation: String?,
)

/** 使用应用内置 sing-box Schema 提供字段名、枚举值和约束说明，不访问网络。 */
class SingBoxSchemaCompletionProvider private constructor(
    private val schemaProvider: () -> String,
) : CompletionProvider {
    constructor(context: Context) : this({
        context.assets.open(SCHEMA_ASSET).bufferedReader().use { it.readText() }
    })

    internal constructor(schemaContent: String) : this({ schemaContent })

    private val navigator by lazy(LazyThreadSafetyMode.SYNCHRONIZED) {
        SchemaNavigator(singBoxSchemaJson.parseToJsonElement(schemaProvider()).jsonObject)
    }

    override suspend fun complete(request: CompletionRequest): CompletionResult? =
        withContext(Dispatchers.Default) {
            val context =
                JsonCursorAnalyzer.analyze(request.text, request.caret) ?: return@withContext null
            if (!request.explicit && !context.quoted && context.kind == CursorKind.Property) {
                return@withContext null
            }

            val items = when (context.kind) {
                CursorKind.Property -> propertyItems(context)
                CursorKind.Value -> valueItems(context)
            }
            if (items.isEmpty() ||
                (context.quoted && items.size == 1 && items.single().label == context.prefix)
            ) {
                return@withContext null
            }

            CompletionResult(
                replaceRange = TextRange(
                    offsetToPosition(request.text, context.replaceStart),
                    offsetToPosition(request.text, context.replaceEnd),
                ),
                items = items,
            )
        }

    fun contextHelp(text: String, caret: TextPosition): SingBoxSchemaContextHelp? {
        val context = JsonCursorAnalyzer.analyze(text, caret) ?: return null
        return when (context.kind) {
            CursorKind.Property -> {
                val fields = navigator.properties(context.path, context.discriminators)
                val field = fields[context.prefix] ?: fields.values
                    .filter { it.name.startsWith(context.prefix, ignoreCase = true) }
                    .singleOrNull()
                SingBoxSchemaContextHelp(
                    path = context.path.toDisplayPath(),
                    field = field?.name,
                    documentation = field?.documentation,
                )
            }

            CursorKind.Value -> {
                val valueSchema = navigator.valueSchema(context.path, context.discriminators)
                SingBoxSchemaContextHelp(
                    path = context.path.toDisplayPath(),
                    field = (context.path.lastOrNull() as? JsonPathSegment.Property)?.name,
                    documentation = valueSchema.documentation,
                )
            }
        }
    }

    private fun propertyItems(context: JsonCursorContext): List<CompletionItem> {
        val fields = navigator.properties(context.path, context.discriminators)
        return fields.values.asSequence()
            .filter { field -> field.name !in context.existingKeys }
            .filter { field -> field.name.contains(context.prefix, ignoreCase = true) }
            .sortedWith(
                compareByDescending<SchemaField> {
                    it.name.startsWith(
                        context.prefix,
                        ignoreCase = true
                    )
                }
                    .thenByDescending { it.required }
                    .thenBy { it.name },
            )
            .map { field ->
                CompletionItem(
                    label = field.name,
                    insertText = if (context.quoted) field.name else "${JsonPrimitive(field.name)}: ",
                    detail = buildString {
                        append(field.typeLabel)
                        if (field.required) append(" · 必填")
                    },
                    documentation = field.documentation,
                    kind = CompletionItemKind.Property,
                )
            }
            .toList()
    }

    private fun valueItems(context: JsonCursorContext): List<CompletionItem> {
        val valueSchema = navigator.valueSchema(context.path, context.discriminators)
        val valueItems = valueSchema.values.asSequence()
            .filter { value -> !context.quoted || value is JsonPrimitive && value.isString }
            .map { value ->
                val label = when {
                    value is JsonPrimitive && value.isString -> value.content
                    else -> value.toString()
                }
                value to label
            }
            .filter { (_, label) -> label.contains(context.prefix, ignoreCase = true) }
            .sortedWith(
                compareByDescending<Pair<JsonElement, String>> {
                    it.second.startsWith(context.prefix, ignoreCase = true)
                }.thenBy { it.second },
            )
            .map { (value, label) ->
                CompletionItem(
                    label = label,
                    insertText = if (context.quoted && value is JsonPrimitive && value.isString) {
                        value.content
                    } else {
                        value.toString()
                    },
                    detail = valueTypeLabel(value),
                    documentation = valueSchema.documentation,
                    kind = CompletionItemKind.Value,
                )
            }
            .toList()
        val templateItems = if (context.quoted) {
            emptyList()
        } else {
            valueSchema.templates.asSequence()
                .filter { it.label.contains(context.prefix, ignoreCase = true) }
                .map { template ->
                    CompletionItem(
                        label = template.label,
                        insertText = template.value.toString(),
                        detail = "配置片段",
                        documentation = template.documentation,
                        kind = CompletionItemKind.Keyword,
                    )
                }
                .toList()
        }
        return valueItems + templateItems
    }

    private companion object {
        const val SCHEMA_ASSET = "sing-box.schema.json"
    }
}

private enum class CursorKind { Property, Value }

private sealed interface JsonPathSegment {
    data class Property(val name: String) : JsonPathSegment
    data class Index(val value: Int) : JsonPathSegment
}

private data class JsonCursorContext(
    val kind: CursorKind,
    val path: List<JsonPathSegment>,
    val prefix: String,
    val replaceStart: Int,
    val replaceEnd: Int,
    val quoted: Boolean,
    val existingKeys: Set<String>,
    val discriminators: Map<List<JsonPathSegment>, String>,
)

/** 容忍光标处未闭合字符串和未完成值的轻量 JSON 状态机。 */
private object JsonCursorAnalyzer {
    fun analyze(text: String, caret: TextPosition): JsonCursorContext? {
        val caretOffset = positionToOffset(text, caret)
        val stack = mutableListOf<JsonFrame>()
        val discriminators = linkedMapOf<List<JsonPathSegment>, String>()
        var index = 0

        while (index < caretOffset) {
            val char = text[index]
            if (char.isWhitespace()) {
                index++
                continue
            }

            if (char == '"') {
                val contentStart = index + 1
                var cursor = contentStart
                var escaped = false
                while (cursor < caretOffset) {
                    val current = text[cursor]
                    if (!escaped && current == '"') break
                    escaped = !escaped && current == '\\'
                    if (current != '\\') escaped = false
                    cursor++
                }
                if (cursor >= caretOffset) {
                    return stringContext(
                        stack = stack,
                        rawPrefix = text.substring(contentStart, caretOffset),
                        contentStart = contentStart,
                        caretOffset = caretOffset,
                        discriminators = discriminators,
                    )
                }

                val value = decodeLooseJsonString(text.substring(contentStart, cursor))
                consumeString(stack.lastOrNull(), value, discriminators)
                index = cursor + 1
                continue
            }

            when (char) {
                '{' -> {
                    val path = nextValuePath(stack.lastOrNull()) ?: emptyList()
                    consumeContainerStart(stack.lastOrNull())
                    stack += ObjectFrame(path)
                    index++
                }

                '[' -> {
                    val path = nextValuePath(stack.lastOrNull()) ?: emptyList()
                    consumeContainerStart(stack.lastOrNull())
                    stack += ArrayFrame(path)
                    index++
                }

                '}' -> {
                    if (stack.lastOrNull() !is ObjectFrame) return null
                    stack.removeAt(stack.lastIndex)
                    index++
                }

                ']' -> {
                    if (stack.lastOrNull() !is ArrayFrame) return null
                    stack.removeAt(stack.lastIndex)
                    index++
                }

                ':' -> {
                    val frame = stack.lastOrNull() as? ObjectFrame ?: return null
                    if (frame.state != ObjectState.Colon) return null
                    frame.state = ObjectState.Value
                    index++
                }

                ',' -> {
                    when (val frame = stack.lastOrNull()) {
                        is ObjectFrame -> {
                            if (frame.state != ObjectState.Comma) return null
                            frame.state = ObjectState.Key
                        }

                        is ArrayFrame -> {
                            if (frame.state != ArrayState.Comma) return null
                            frame.state = ArrayState.Value
                        }

                        null -> return null
                    }
                    index++
                }

                else -> {
                    val frame = stack.lastOrNull()
                    if (!isExpectingValue(frame)) return null
                    val tokenStart = index
                    while (index < caretOffset && !text[index].isJsonDelimiter()) index++
                    if (index == caretOffset) {
                        val path = nextValuePath(frame) ?: return null
                        return JsonCursorContext(
                            kind = CursorKind.Value,
                            path = path,
                            prefix = text.substring(tokenStart, caretOffset),
                            replaceStart = tokenStart,
                            replaceEnd = caretOffset,
                            quoted = false,
                            existingKeys = emptySet(),
                            discriminators = discriminators.toMap(),
                        )
                    }
                    consumePrimitive(frame)
                }
            }
        }

        return when (val frame = stack.lastOrNull()) {
            is ObjectFrame -> when (frame.state) {
                ObjectState.Key -> JsonCursorContext(
                    kind = CursorKind.Property,
                    path = frame.path,
                    prefix = "",
                    replaceStart = caretOffset,
                    replaceEnd = caretOffset,
                    quoted = false,
                    existingKeys = frame.keys.toSet(),
                    discriminators = discriminators.toMap(),
                )

                ObjectState.Value -> valueContext(frame, caretOffset, discriminators)
                else -> null
            }

            is ArrayFrame -> if (frame.state == ArrayState.Value) {
                JsonCursorContext(
                    kind = CursorKind.Value,
                    path = frame.path + JsonPathSegment.Index(frame.index),
                    prefix = "",
                    replaceStart = caretOffset,
                    replaceEnd = caretOffset,
                    quoted = false,
                    existingKeys = emptySet(),
                    discriminators = discriminators.toMap(),
                )
            } else {
                null
            }

            null -> null
        }
    }

    private fun stringContext(
        stack: List<JsonFrame>,
        rawPrefix: String,
        contentStart: Int,
        caretOffset: Int,
        discriminators: Map<List<JsonPathSegment>, String>,
    ): JsonCursorContext? = when (val frame = stack.lastOrNull()) {
        is ObjectFrame -> when (frame.state) {
            ObjectState.Key -> JsonCursorContext(
                kind = CursorKind.Property,
                path = frame.path,
                prefix = decodeLooseJsonString(rawPrefix),
                replaceStart = contentStart,
                replaceEnd = caretOffset,
                quoted = true,
                existingKeys = frame.keys.toSet(),
                discriminators = discriminators.toMap(),
            )

            ObjectState.Value -> valueContext(
                frame = frame,
                caretOffset = caretOffset,
                discriminators = discriminators,
                prefix = decodeLooseJsonString(rawPrefix),
                replaceStart = contentStart,
                quoted = true,
            )

            else -> null
        }

        is ArrayFrame -> if (frame.state == ArrayState.Value) {
            JsonCursorContext(
                kind = CursorKind.Value,
                path = frame.path + JsonPathSegment.Index(frame.index),
                prefix = decodeLooseJsonString(rawPrefix),
                replaceStart = contentStart,
                replaceEnd = caretOffset,
                quoted = true,
                existingKeys = emptySet(),
                discriminators = discriminators.toMap(),
            )
        } else {
            null
        }

        null -> null
    }

    private fun valueContext(
        frame: ObjectFrame,
        caretOffset: Int,
        discriminators: Map<List<JsonPathSegment>, String>,
        prefix: String = "",
        replaceStart: Int = caretOffset,
        quoted: Boolean = false,
    ): JsonCursorContext? {
        val key = frame.pendingKey ?: return null
        return JsonCursorContext(
            kind = CursorKind.Value,
            path = frame.path + JsonPathSegment.Property(key),
            prefix = prefix,
            replaceStart = replaceStart,
            replaceEnd = caretOffset,
            quoted = quoted,
            existingKeys = emptySet(),
            discriminators = discriminators.toMap(),
        )
    }

    private fun consumeString(
        frame: JsonFrame?,
        value: String,
        discriminators: MutableMap<List<JsonPathSegment>, String>,
    ) {
        when (frame) {
            is ObjectFrame -> when (frame.state) {
                ObjectState.Key -> {
                    frame.pendingKey = value
                    frame.keys += value
                    frame.state = ObjectState.Colon
                }

                ObjectState.Value -> {
                    if (frame.pendingKey == "type") discriminators[frame.path] = value
                    finishObjectValue(frame)
                }

                else -> Unit
            }

            is ArrayFrame -> if (frame.state == ArrayState.Value) finishArrayValue(frame)
            null -> Unit
        }
    }

    private fun consumeContainerStart(frame: JsonFrame?) {
        when (frame) {
            is ObjectFrame -> if (frame.state == ObjectState.Value) finishObjectValue(frame)
            is ArrayFrame -> if (frame.state == ArrayState.Value) finishArrayValue(frame)
            null -> Unit
        }
    }

    private fun consumePrimitive(frame: JsonFrame?) {
        when (frame) {
            is ObjectFrame -> finishObjectValue(frame)
            is ArrayFrame -> finishArrayValue(frame)
            null -> Unit
        }
    }

    private fun finishObjectValue(frame: ObjectFrame) {
        frame.pendingKey = null
        frame.state = ObjectState.Comma
    }

    private fun finishArrayValue(frame: ArrayFrame) {
        frame.index++
        frame.state = ArrayState.Comma
    }

    private fun nextValuePath(frame: JsonFrame?): List<JsonPathSegment>? = when (frame) {
        is ObjectFrame -> frame.pendingKey?.let { frame.path + JsonPathSegment.Property(it) }
        is ArrayFrame -> frame.path + JsonPathSegment.Index(frame.index)
        null -> emptyList()
    }

    private fun isExpectingValue(frame: JsonFrame?): Boolean = when (frame) {
        is ObjectFrame -> frame.state == ObjectState.Value
        is ArrayFrame -> frame.state == ArrayState.Value
        null -> true
    }
}

private sealed class JsonFrame(open val path: List<JsonPathSegment>)

private class ObjectFrame(
    override val path: List<JsonPathSegment>,
    var state: ObjectState = ObjectState.Key,
    var pendingKey: String? = null,
    val keys: MutableSet<String> = linkedSetOf(),
) : JsonFrame(path)

private class ArrayFrame(
    override val path: List<JsonPathSegment>,
    var state: ArrayState = ArrayState.Value,
    var index: Int = 0,
) : JsonFrame(path)

private enum class ObjectState { Key, Colon, Value, Comma }
private enum class ArrayState { Value, Comma }

private data class SchemaField(
    val name: String,
    val typeLabel: String,
    val required: Boolean,
    val documentation: String?,
)

private data class SchemaValue(
    val values: List<JsonElement>,
    val templates: List<SchemaTemplate>,
    val documentation: String?,
)

private data class SchemaTemplate(
    val label: String,
    val value: JsonObject,
    val documentation: String?,
)

/** 只解析本地 $ref；内置 Schema 不包含远程引用。 */
private class SchemaNavigator(private val root: JsonObject) {
    private val references = SingBoxSchemaReferenceResolver(root)
    fun properties(
        path: List<JsonPathSegment>,
        discriminators: Map<List<JsonPathSegment>, String>,
    ): Map<String, SchemaField> {
        val schemas = schemasAt(path, discriminators)
        val propertySchemas = linkedMapOf<String, MutableList<JsonObject>>()
        val required = linkedSetOf<String>()
        schemas.forEach { schema ->
            collectProperties(
                schema = schema,
                discriminator = discriminators[path],
                properties = propertySchemas,
                required = required,
            )
        }
        return propertySchemas.mapValues { (name, values) ->
            val schemas = values.distinct()
            SchemaField(
                name = name,
                typeLabel = schemaTypes(schemas).joinToString(" | ").ifBlank { "任意" },
                required = name in required,
                documentation = combineDocumentation(
                    commonFieldDocumentation(name),
                    schemaDocumentation(schemas, name in required),
                ),
            )
        }
    }

    fun valueSchema(
        path: List<JsonPathSegment>,
        discriminators: Map<List<JsonPathSegment>, String>,
    ): SchemaValue {
        val schemas = schemasAt(path, discriminators)
        val values = linkedMapOf<String, JsonElement>()
        schemas.forEach { collectValues(it, values) }
        if (values.isEmpty() && "布尔值" in schemaTypes(schemas)) {
            listOf(JsonPrimitive(true), JsonPrimitive(false)).forEach { values[it.toString()] = it }
        }
        return SchemaValue(
            values = values.values.toList(),
            templates = objectTemplates(schemas),
            documentation = combineDocumentation(
                (path.lastOrNull() as? JsonPathSegment.Property)?.name
                    ?.let(::commonFieldDocumentation),
                schemaDocumentation(schemas, required = false),
            ),
        )
    }

    private fun objectTemplates(schemas: List<JsonObject>): List<SchemaTemplate> {
        val templates = linkedMapOf<String, SchemaTemplate>()
        schemas.forEach { schema ->
            collectTemplateBranches(schema).forEach { branch ->
                val discriminator = discriminatorConst(branch) ?: return@forEach
                val (key, value) = discriminator
                val labelValue = if (value is JsonPrimitive && value.isString) {
                    value.content
                } else {
                    value.toString()
                }
                val template = SchemaTemplate(
                    label = "$labelValue $key",
                    value = JsonObject(mapOf(key to value)),
                    documentation = combineDocumentation(
                        commonFieldDocumentation(key),
                        schemaDocumentation(listOf(branch), required = false),
                    ),
                )
                templates[template.value.toString()] = template
            }
        }
        return templates.values.toList()
    }

    private fun collectTemplateBranches(
        schema: JsonObject,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ): List<JsonObject> {
        if (depth > MAX_SCHEMA_DEPTH) return emptyList()
        val branches = mutableListOf<JsonObject>()
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            branches += collectTemplateBranches(target, visitedRefs + ref, depth + 1)
        }
        listOf("oneOf", "anyOf").forEach { keyword ->
            schema[keyword]?.asArray()?.forEach { child ->
                child.asObject()?.let {
                    branches += collectTemplateBranches(it, visitedRefs, depth + 1)
                }
            }
        }
        if (branches.isEmpty() && discriminatorConst(schema) != null) branches += schema
        return branches
    }

    private fun discriminatorConst(
        schema: JsonObject,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ): Pair<String, JsonElement>? {
        if (depth > MAX_SCHEMA_DEPTH) return null
        val properties = schema["properties"]?.asObject()
        listOf("type", "action").forEach { key ->
            properties?.get(key)?.asObject()?.get("const")?.let { return key to it }
        }
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            discriminatorConst(target, visitedRefs + ref, depth + 1)?.let { return it }
        }
        schema["allOf"]?.asArray()?.forEach { child ->
            child.asObject()?.let {
                discriminatorConst(it, visitedRefs, depth + 1)
            }?.let { return it }
        }
        return null
    }

    private fun schemasAt(
        path: List<JsonPathSegment>,
        discriminators: Map<List<JsonPathSegment>, String>,
    ): List<JsonObject> {
        var current = listOf(root)
        var traversed = emptyList<JsonPathSegment>()
        path.forEach { segment ->
            val discriminator = discriminators[traversed]
            current = when (segment) {
                is JsonPathSegment.Property -> current.flatMap {
                    propertySchemas(it, segment.name, discriminator)
                }

                is JsonPathSegment.Index -> current.flatMap { itemSchemas(it, discriminator) }
            }.distinct()
            traversed = traversed + segment
        }
        return current
    }

    private fun collectProperties(
        schema: JsonObject,
        discriminator: String?,
        properties: MutableMap<String, MutableList<JsonObject>>,
        required: MutableSet<String>,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ) {
        if (depth > MAX_SCHEMA_DEPTH) return
        schema["required"]?.asArray()?.forEach { required += it.jsonPrimitive.content }
        schema["properties"]?.asObject()?.forEach { (name, value) ->
            value.asObject()?.let { properties.getOrPut(name) { mutableListOf() } += it }
        }
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            collectProperties(
                target,
                discriminator,
                properties,
                required,
                visitedRefs + ref,
                depth + 1
            )
        }
        schema["allOf"]?.asArray()?.forEach { child ->
            child.asObject()?.let {
                collectProperties(it, discriminator, properties, required, visitedRefs, depth + 1)
            }
        }
        selectedBranches(schema, discriminator).forEach {
            collectProperties(it, discriminator, properties, required, visitedRefs, depth + 1)
        }
    }

    private fun propertySchemas(
        schema: JsonObject,
        name: String,
        discriminator: String?,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ): List<JsonObject> {
        if (depth > MAX_SCHEMA_DEPTH) return emptyList()
        val result = mutableListOf<JsonObject>()
        schema["properties"]?.asObject()?.get(name)?.asObject()?.let(result::add)
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            result += propertySchemas(target, name, discriminator, visitedRefs + ref, depth + 1)
        }
        schema["allOf"]?.asArray()?.forEach { child ->
            child.asObject()?.let {
                result += propertySchemas(it, name, discriminator, visitedRefs, depth + 1)
            }
        }
        selectedBranches(schema, discriminator).forEach {
            result += propertySchemas(it, name, discriminator, visitedRefs, depth + 1)
        }
        return result
    }

    private fun itemSchemas(
        schema: JsonObject,
        discriminator: String?,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ): List<JsonObject> {
        if (depth > MAX_SCHEMA_DEPTH) return emptyList()
        val result = mutableListOf<JsonObject>()
        schema["items"]?.asObject()?.let(result::add)
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            result += itemSchemas(target, discriminator, visitedRefs + ref, depth + 1)
        }
        schema["allOf"]?.asArray()?.forEach { child ->
            child.asObject()?.let {
                result += itemSchemas(it, discriminator, visitedRefs, depth + 1)
            }
        }
        selectedBranches(schema, discriminator).forEach {
            result += itemSchemas(it, discriminator, visitedRefs, depth + 1)
        }
        return result
    }

    private fun collectValues(
        schema: JsonObject,
        values: MutableMap<String, JsonElement>,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ) {
        if (depth > MAX_SCHEMA_DEPTH) return
        schema["const"]?.let { values[it.toString()] = it }
        schema["enum"]?.asArray()?.forEach { values[it.toString()] = it }
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            collectValues(target, values, visitedRefs + ref, depth + 1)
        }
        listOf("oneOf", "anyOf", "allOf").forEach { keyword ->
            schema[keyword]?.asArray()?.forEach { child ->
                child.asObject()?.let { collectValues(it, values, visitedRefs, depth + 1) }
            }
        }
    }

    private fun schemaTypes(schemas: List<JsonObject>): Set<String> {
        val result = linkedSetOf<String>()
        schemas.forEach { collectSchemaTypes(it, result) }
        return result
    }

    private fun collectSchemaTypes(
        schema: JsonObject,
        result: MutableSet<String>,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ) {
        if (depth > MAX_SCHEMA_DEPTH) return
        when (val type = schema["type"]) {
            is JsonPrimitive -> type.contentOrNull?.let { result += localizedType(it) }
            is JsonArray -> type.forEach {
                it.asPrimitive()?.contentOrNull?.let { raw -> result += localizedType(raw) }
            }

            else -> Unit
        }
        schema["const"]?.let { result += valueTypeLabel(it) }
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            collectSchemaTypes(target, result, visitedRefs + ref, depth + 1)
        }
        listOf("oneOf", "anyOf", "allOf").forEach { keyword ->
            schema[keyword]?.asArray()?.forEach { child ->
                child.asObject()?.let { collectSchemaTypes(it, result, visitedRefs, depth + 1) }
            }
        }
    }

    private fun schemaDocumentation(schemas: List<JsonObject>, required: Boolean): String? {
        val parts = linkedSetOf<String>()
        if (required) parts += "必填字段。"
        schemas.forEach { collectSchemaDocumentation(it, parts) }
        return parts.joinToString(" ").ifBlank { null }
    }

    private fun collectSchemaDocumentation(
        schema: JsonObject,
        parts: MutableSet<String>,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ) {
        if (depth > MAX_SCHEMA_DEPTH) return
        schema["description"]?.asPrimitive()?.contentOrNull?.takeIf(String::isNotBlank)
            ?.let(parts::add)
        schema["default"]?.let { parts += "默认值：$it。" }
        schema["enum"]?.asArray()?.takeIf { it.isNotEmpty() }?.let { values ->
            parts += "可选值：${values.joinToString { valueLabel(it) }}。"
        }
        schema["minimum"]?.let { parts += "最小值：$it。" }
        schema["maximum"]?.let { parts += "最大值：$it。" }
        schema["pattern"]?.asPrimitive()?.contentOrNull?.let { parts += "格式：$it。" }
        schema["x-tag-reference"]?.asPrimitive()?.contentOrNull?.let { parts += "引用 $it 标签。" }
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            collectSchemaDocumentation(target, parts, visitedRefs + ref, depth + 1)
        }
        listOf("oneOf", "anyOf", "allOf").forEach { keyword ->
            schema[keyword]?.asArray()?.forEach { child ->
                child.asObject()?.let {
                    collectSchemaDocumentation(it, parts, visitedRefs, depth + 1)
                }
            }
        }
    }

    private fun selectedBranches(schema: JsonObject, discriminator: String?): List<JsonObject> {
        val branches = (schema["oneOf"] ?: schema["anyOf"])
            ?.asArray()
            ?.mapNotNull(JsonElement::asObject)
            .orEmpty()
        if (branches.isEmpty() || discriminator == null) return branches
        return branches.filter { branch -> typeConst(branch) == discriminator }.ifEmpty { branches }
    }

    private fun typeConst(
        schema: JsonObject,
        visitedRefs: Set<String> = emptySet(),
        depth: Int = 0,
    ): String? {
        if (depth > MAX_SCHEMA_DEPTH) return null
        schema["properties"]?.asObject()?.get("type")?.asObject()
            ?.get("const")?.asPrimitive()?.contentOrNull?.let { return it }
        referencedSchema(schema, visitedRefs)?.let { (ref, target) ->
            typeConst(target, visitedRefs + ref, depth + 1)?.let { return it }
        }
        schema["allOf"]?.asArray()?.forEach { child ->
            child.asObject()?.let { typeConst(it, visitedRefs, depth + 1) }?.let { return it }
        }
        return null
    }

    private fun referencedSchema(
        schema: JsonObject,
        visitedRefs: Set<String>,
    ): Pair<String, JsonObject>? = references.referencedSchema(schema, visitedRefs)

    private companion object {
        const val MAX_SCHEMA_DEPTH = 32
    }
}

private fun valueTypeLabel(value: JsonElement): String = when (value) {
    is JsonObject -> "对象"
    is JsonArray -> "数组"
    is JsonPrimitive -> when {
        value.isString -> "字符串"
        value.booleanOrNull != null -> "布尔值"
        value.contentOrNull == "null" -> "空值"
        else -> "数字"
    }
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

private fun valueLabel(value: JsonElement): String =
    if (value is JsonPrimitive && value.isString) value.content else value.toString()

private fun List<JsonPathSegment>.toDisplayPath(): String = buildString {
    append('$')
    this@toDisplayPath.forEach { segment ->
        when (segment) {
            is JsonPathSegment.Property -> append('.').append(segment.name)
            is JsonPathSegment.Index -> append('[').append(segment.value).append(']')
        }
    }
}

private fun combineDocumentation(vararg values: String?): String? = values
    .filterNotNull()
    .map(String::trim)
    .filter(String::isNotEmpty)
    .distinct()
    .joinToString(" ")
    .ifBlank { null }

private fun commonFieldDocumentation(name: String): String? = when (name) {
    "type" -> "配置对象的类型；选择后，补全列表会只显示该类型支持的字段。"
    "tag" -> "该对象的唯一名称，供其他配置通过标签引用。"
    "enabled" -> "控制当前功能是否启用。"
    "server" -> "远程服务器地址，可以是域名或 IP 地址。"
    "server_port" -> "远程服务器端口。"
    "listen" -> "本地监听地址。"
    "listen_port" -> "本地监听端口。"
    "outbound" -> "命中后使用的出站标签。"
    "default_domain_resolver" -> "解析服务器域名时使用的 DNS 服务器标签。"
    "rule_set" -> "匹配一个或多个规则集标签。"
    "rules" -> "按顺序匹配的规则列表。"
    "action" -> "规则命中后执行的动作。"
    "servers" -> "当前组包含的服务器或成员标签。"
    "url" -> "下载、健康检查或延迟测试使用的 URL。"
    "interval" -> "自动更新或测试的时间间隔，例如 3m、1h。"
    "path" -> "本地文件或资源路径。"
    "initial_path" -> "首次启动时使用的本地资源路径。"
    "http_client" -> "执行远程请求时使用的 HTTP Client 标签。"
    "secret" -> "访问控制接口时使用的鉴权密钥。"
    else -> null
}

private fun JsonElement.asObject(): JsonObject? = this as? JsonObject
private fun JsonElement.asArray(): JsonArray? = this as? JsonArray
private fun JsonElement.asPrimitive(): JsonPrimitive? = this as? JsonPrimitive

private fun Char.isJsonDelimiter(): Boolean =
    isWhitespace() || this == ',' || this == '}' || this == ']'

private fun decodeLooseJsonString(raw: String): String = runCatching {
    singBoxSchemaJson.parseToJsonElement("\"$raw\"").jsonPrimitive.content
}.getOrElse {
    raw.replace("\\\"", "\"").replace("\\\\", "\\")
}

private fun positionToOffset(text: String, position: TextPosition): Int {
    val targetLine = position.line.coerceAtLeast(0)
    var line = 0
    var lineStart = 0
    var index = 0
    while (index < text.length && line < targetLine) {
        if (text[index] == '\n') {
            line++
            lineStart = index + 1
        }
        index++
    }
    val lineEnd = text.indexOf('\n', lineStart).let { if (it < 0) text.length else it }
    return (lineStart + position.column.coerceAtLeast(0)).coerceAtMost(lineEnd)
}

private fun offsetToPosition(text: String, rawOffset: Int): TextPosition {
    val position = sourcePositionAt(text, rawOffset)
    return TextPosition(position.line - 1, position.column - 1)
}
