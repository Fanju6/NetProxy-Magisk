package com.fanjv.netproxy.feature.routing.model

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull

internal enum class LocalRuleSet(val tag: String) {
    Proxy("proxy"), Direct("direct"), Block("block"), Policy("route");

    val target: String get() = if (this == Policy) "singbox/route" else "singbox/rules/local/$tag.json"

    companion object {
        val shortcuts = entries.filterNot { it == Policy }
    }
}

internal enum class RuleField(val key: String) {
    DomainSuffix("domain_suffix"), Domain("domain"), DomainKeyword("domain_keyword"),
    DomainRegex("domain_regex"), IpCidr("ip_cidr"), Port("port"), PortRange("port_range"),
    PackageName("package_name");

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
            this == PackageName && values.any { !packageName.matches(it) } -> RuleInputError.PackageName
            else -> null
        }
    }

    fun rule(text: String): JsonObject {
        require(inputError(text) == null)
        return JsonObject(mapOf(key to JsonArray(values(text).map {
            if (this == Port) JsonPrimitive(it.toInt()) else JsonPrimitive(it)
        })))
    }

    companion object {
        private val packageName = Regex("[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z][A-Za-z0-9_]*)*")
    }
}

internal enum class RuleInputError { Empty, Domain, Port, PortRange, PackageName }
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

internal data class LocalRuleDocument(val root: JsonObject, private val nativeRoute: Boolean = false) {
    private val section: JsonObject get() = if (nativeRoute) root.getValue("route") as JsonObject else root
    val rules: JsonArray get() = section["rules"] as? JsonArray ?: JsonArray(emptyList())

    val automaticPolicyInsertion: Int? get() {
        if (!nativeRoute) return null
        if (rules.all { it.policyRule() != null }) return rules.size
        // 只为能识别出默认优先级布局的配置推断位置，不用广告资源名定位。
        val prefix = rules.takeWhile(::priorityRule)
        if (!globalModes.all { mode -> prefix.any { modeRoute(it as JsonObject, mode) } } ||
            prefix.none { (it as JsonObject)["action"] == hijackDns }) return null
        val rest = rules.drop(prefix.size)
        if (rest.any(::priorityRule)) return null
        val insertion = prefix.size + rest.takeWhile { it.policyRule() != null }.size
        return insertion.takeIf { rules.getOrNull(it)?.let(::siteRule) != false }
    }

    val editableIndices: List<Int> get() =
        rules.indices.filter { !nativeRoute || rules[it].policyRule() != null }

    fun earlierCatchAll(insertion: Int): Int? =
        rules.take(insertion).indexOfFirst { value ->
            val rule = value as? JsonObject ?: return@indexOfFirst false
            val action = (rule["action"] as? JsonPrimitive)?.content ?: "route"
            val modes = rule["clash_mode"].values()
            (rule["type"] == null || rule["type"] == JsonPrimitive("default")) &&
                action in setOf("route", "reject") &&
                rule.keys.all { it in setOf("type", "action", "outbound", "method", "no_drop", "clash_mode") } &&
                (modes.isEmpty() || modes.any { it !in globalModes })
        }.takeIf { it >= 0 }

    fun replace(index: Int?, rule: JsonObject, insertion: Int? = null): LocalRuleDocument {
        val updated = rules.toMutableList()
        if (index == null) {
            val position = if (nativeRoute) requireNotNull(insertion ?: automaticPolicyInsertion) else updated.size
            updated.add(position, rule)
        } else updated[index] = rule
        return withRules(updated)
    }

    fun remove(index: Int): LocalRuleDocument = withRules(rules.toMutableList().apply { removeAt(index) })

    private fun withRules(rules: List<JsonElement>): LocalRuleDocument {
        val updated = JsonObject(section + ("rules" to JsonArray(rules)))
        return copy(root = if (nativeRoute) JsonObject(root + ("route" to updated)) else updated)
    }
    fun content(): String = json.encodeToString(root)

    companion object {
        private val json = Json { prettyPrint = true; prettyPrintIndent = "  " }
        private val hijackDns = JsonPrimitive("hijack-dns")
        private val protectedActions = setOf(JsonPrimitive("sniff"), hijackDns, JsonPrimitive("bypass"))
        private val globalModes = setOf(JsonPrimitive("Direct"), JsonPrimitive("Proxy"))
        private val shortcutTags = setOf(JsonPrimitive("proxy"), JsonPrimitive("direct"), JsonPrimitive("block"))
        private val allowAdsMode = JsonPrimitive("RuleAllowAds")

        private fun modeRoute(rule: JsonObject, mode: JsonPrimitive): Boolean =
            rule["clash_mode"] == mode &&
                (rule["type"] == null || rule["type"] == JsonPrimitive("default")) &&
                (rule["action"] == null || rule["action"] == JsonPrimitive("route")) &&
                rule["outbound"] is JsonPrimitive &&
                rule.keys.all { it in setOf("type", "action", "outbound", "clash_mode") }

        private fun priorityRule(value: JsonElement): Boolean {
            val rule = value as? JsonObject ?: return false
            return rule["action"] in protectedActions ||
                rule["clash_mode"].values().any { it in globalModes } ||
                rule["rule_set"].values().any { it in shortcutTags } ||
                (rule["action"] == JsonPrimitive("reject") && rule["mode"] == JsonPrimitive("and") &&
                    (rule["rules"] as? JsonArray).orEmpty().any {
                    val child = it as? JsonObject
                    child?.get("rule_set").values().any { tag -> tag == JsonPrimitive("block") } ||
                        (child?.get("clash_mode") == allowAdsMode && child["invert"] == JsonPrimitive(true))
                    })
        }

        private fun siteRule(value: JsonElement): Boolean {
            val rule = value as? JsonObject ?: return false
            return rule.keys.all { it in setOf("type", "action", "outbound", "rule_set") } &&
                (rule["type"] == null || rule["type"] == JsonPrimitive("default")) &&
                (rule["action"] == null || rule["action"] == JsonPrimitive("route")) &&
                rule["rule_set"].values().let { tags ->
                    tags.isNotEmpty() && tags.all {
                        val tag = (it as? JsonPrimitive)?.content.orEmpty()
                        tag.startsWith("geosite/") || tag.startsWith("geoip/")
                    }
                }
        }

        private fun JsonElement?.values(): List<JsonElement> = when (this) {
            null -> emptyList()
            is JsonArray -> this
            else -> listOf(this)
        }

        fun parse(content: String, nativeRoute: Boolean = false): LocalRuleDocument {
            val root = json.parseToJsonElement(content) as? JsonObject ?: error("Invalid rule-set object")
            if (nativeRoute) {
                val route = root["route"] as? JsonObject ?: error("Invalid route object")
                require(route["rules"] == null || route["rules"] is JsonArray)
                return LocalRuleDocument(root, nativeRoute = true)
            }
            require((root["version"] as? JsonPrimitive)?.takeUnless { it.isString }?.intOrNull in 1..5)
            require(root["rules"] is JsonArray)
            return LocalRuleDocument(root)
        }
    }
}

internal fun JsonElement.policyRule(): Pair<String, SimpleRule>? {
    val rule = this as? JsonObject ?: return null
    if (rule["type"] != JsonPrimitive("default") || rule["action"] != JsonPrimitive("route")) return null
    val outbound = (rule["outbound"] as? JsonPrimitive)?.takeIf { it.isString }?.content
        ?.takeIf { it.isNotBlank() } ?: return null
    val match = JsonObject(rule - setOf("action", "outbound")).simpleRule() ?: return null
    return outbound to match
}

internal fun RuleField.policyRule(text: String, outbound: String): JsonObject =
    JsonObject(rule(text) + mapOf(
        "type" to JsonPrimitive("default"),
        "action" to JsonPrimitive("route"),
        "outbound" to JsonPrimitive(outbound),
    ))
