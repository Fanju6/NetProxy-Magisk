package com.fanjv.netproxy.feature.kernel.presentation

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

/** sing-box Schema 与编辑器共用的 JSON 解析器。 */
internal val singBoxSchemaJson = Json { ignoreUnknownKeys = true }

/** JSON 文本中的一基行列位置，供补全和校验结果统一使用。 */
internal data class JsonSourcePosition(
    val line: Int,
    val column: Int,
)

/** 仅解析内置 Schema 的本地 JSON Pointer 引用，不允许访问网络。 */
internal class SingBoxSchemaReferenceResolver(
    private val root: JsonObject,
) {
    fun referencedSchema(
        schema: JsonObject,
        visitedRefs: Set<String>,
    ): Pair<String, JsonObject>? {
        val ref = schema["\$ref"]?.jsonPrimitive?.contentOrNull ?: return null
        if (ref in visitedRefs || !ref.startsWith("#/")) return null

        var target: JsonElement = root
        ref.removePrefix("#/").split('/').forEach { rawSegment ->
            val segment = rawSegment.replace("~1", "/").replace("~0", "~")
            target = (target as? JsonObject)?.get(segment) ?: return null
        }
        return ref to (target as? JsonObject ?: return null)
    }
}

/** 将字符偏移转换为一基行列位置。 */
internal fun sourcePositionAt(text: String, rawOffset: Int): JsonSourcePosition {
    val offset = rawOffset.coerceIn(0, text.length)
    var line = 1
    var lineStart = 0
    for (index in 0 until offset) {
        if (text[index] == '\n') {
            line++
            lineStart = index + 1
        }
    }
    return JsonSourcePosition(line = line, column = offset - lineStart + 1)
}

/**
 * 为有效 JSON 构建 JSON Pointer 到值起始位置的索引。
 *
 * `kotlinx.serialization` 负责语法解析；此扫描器只保留位置，避免为 Android
 * 引入完整 JSON Schema 引擎及其 Jackson 依赖。
 */
internal fun buildJsonSourceIndex(text: String): Map<String, JsonSourcePosition> =
    runCatching { JsonSourceIndexer(text).build() }.getOrDefault(emptyMap())

private class JsonSourceIndexer(
    private val text: String,
) {
    private val positions = linkedMapOf<String, JsonSourcePosition>()
    private var index = 0

    fun build(): Map<String, JsonSourcePosition> {
        skipWhitespace()
        parseValue("")
        return positions
    }

    private fun parseValue(path: String) {
        skipWhitespace()
        positions.putIfAbsent(path, sourcePositionAt(text, index))
        when (peek()) {
            '{' -> parseObject(path)
            '[' -> parseArray(path)
            '"' -> consumeString()
            else -> consumePrimitive()
        }
    }

    private fun parseObject(path: String) {
        index++
        skipWhitespace()
        if (consumeIf('}')) return

        while (index < text.length) {
            skipWhitespace()
            val key = consumeString()
            skipWhitespace()
            require(consumeIf(':')) { "对象字段缺少冒号" }
            parseValue("$path/${escapePointerSegment(key)}")
            skipWhitespace()
            if (consumeIf('}')) return
            require(consumeIf(',')) { "对象字段缺少逗号" }
        }
        error("对象未闭合")
    }

    private fun parseArray(path: String) {
        index++
        skipWhitespace()
        if (consumeIf(']')) return

        var itemIndex = 0
        while (index < text.length) {
            parseValue("$path/$itemIndex")
            itemIndex++
            skipWhitespace()
            if (consumeIf(']')) return
            require(consumeIf(',')) { "数组元素缺少逗号" }
        }
        error("数组未闭合")
    }

    private fun consumeString(): String {
        val start = index
        require(consumeIf('"')) { "字符串应以引号开始" }
        var escaped = false
        while (index < text.length) {
            val current = text[index++]
            if (!escaped && current == '"') {
                val literal = text.substring(start, index)
                return singBoxSchemaJson.parseToJsonElement(literal).jsonPrimitive.content
            }
            escaped = !escaped && current == '\\'
            if (current != '\\') escaped = false
        }
        error("字符串未闭合")
    }

    private fun consumePrimitive() {
        val start = index
        while (index < text.length && text[index] !in ",]}" && !text[index].isWhitespace()) {
            index++
        }
        require(index > start) { "缺少 JSON 值" }
    }

    private fun skipWhitespace() {
        while (index < text.length && text[index].isWhitespace()) index++
    }

    private fun peek(): Char? = text.getOrNull(index)

    private fun consumeIf(expected: Char): Boolean {
        if (peek() != expected) return false
        index++
        return true
    }
}

private fun escapePointerSegment(value: String): String =
    value.replace("~", "~0").replace("/", "~1")
