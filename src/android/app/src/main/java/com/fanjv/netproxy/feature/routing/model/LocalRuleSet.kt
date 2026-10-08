package com.fanjv.netproxy.feature.routing.model

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull

internal enum class LocalRuleSet(val tag: String) {
    Proxy("proxy"), Direct("direct"), Block("block");

    val target: String get() = "singbox/rules/local/$tag.json"
}

internal enum class RuleField(val key: String) {
    DomainSuffix("domain_suffix"), Domain("domain"), DomainKeyword("domain_keyword"),
    DomainRegex("domain_regex"), IpCidr("ip_cidr"), Port("port"), PortRange("port_range");

    fun values(text: String): List<String> = text.lines().map(String::trim).filter(String::isNotEmpty).distinct()

    fun inputError(text: String): RuleInputError? {
        val values = values(text)
        return when {
            values.isEmpty() -> RuleInputError.Empty
            this == Port && values.any { it.toIntOrNull()?.let { port -> port in 1..65535 } != true } -> RuleInputError.Port
            this == PortRange && values.any { value ->
                val bounds = value.split(':')
                val start = bounds.getOrNull(0)?.toIntOrNull()
                val end = bounds.getOrNull(1)?.toIntOrNull()
                bounds.size != 2 || start == null || end == null || start !in 1..65535 || end !in start..65535
            } -> RuleInputError.PortRange
            this in setOf(Domain, DomainSuffix) && values.any { value ->
                value.any(Char::isWhitespace) || value.any { it in "/:@" }
            } -> RuleInputError.Domain
            else -> null
        }
    }

    fun rule(text: String): JsonObject {
        require(inputError(text) == null)
        return JsonObject(mapOf(key to JsonArray(values(text).map {
            if (this == Port) JsonPrimitive(it.toInt()) else JsonPrimitive(it)
        })))
    }
}

internal enum class RuleInputError { Empty, Domain, Port, PortRange }
internal data class SimpleRule(val field: RuleField, val values: List<String>)

internal fun JsonElement.simpleRule(): SimpleRule? {
    val rule = this as? JsonObject ?: return null
    val type = rule["type"]
    if (type != null && type != JsonPrimitive("default")) return null
    val field = RuleField.entries.singleOrNull { it.key in rule } ?: return null
    // 多条件、取反及其他原生选项必须交给 JSON 编辑器，不能简化后改变匹配语义。
    if (rule.keys.any { it != field.key && it != "type" }) return null
    val values = when (val value = rule[field.key]) {
        is JsonArray -> value
        is JsonPrimitive -> listOf(value)
        else -> return null
    }
    if (values.isEmpty()) return null
    val strings = values.map { value ->
        val primitive = value as? JsonPrimitive ?: return null
        if (field == RuleField.Port) {
            if (primitive.isString || primitive.intOrNull !in 1..65535) return null
        } else if (!primitive.isString || primitive.content.isBlank() || '\n' in primitive.content || '\r' in primitive.content) {
            return null
        }
        primitive.content
    }
    return SimpleRule(field, strings)
}

internal data class LocalRuleDocument(val root: JsonObject) {
    val rules: JsonArray get() = root.getValue("rules") as JsonArray

    fun replace(index: Int?, rule: JsonObject): LocalRuleDocument {
        val updated = rules.toMutableList()
        if (index == null) updated.add(rule) else updated[index] = rule
        return withRules(updated)
    }

    fun remove(index: Int): LocalRuleDocument = withRules(rules.toMutableList().apply { removeAt(index) })

    private fun withRules(rules: List<JsonElement>) = LocalRuleDocument(JsonObject(root + ("rules" to JsonArray(rules))))
    fun content(): String = json.encodeToString(root)

    companion object {
        private val json = Json { prettyPrint = true; prettyPrintIndent = "  " }

        fun parse(content: String): LocalRuleDocument {
            val root = json.parseToJsonElement(content) as? JsonObject ?: error("Invalid rule-set object")
            require((root["version"] as? JsonPrimitive)?.takeUnless { it.isString }?.intOrNull in 1..5)
            require(root["rules"] is JsonArray)
            return LocalRuleDocument(root)
        }
    }
}
