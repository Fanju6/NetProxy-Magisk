package com.fanjv.netproxy.feature.routing

import com.fanjv.netproxy.feature.routing.model.LocalRuleDocument
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.RuleInputError
import com.fanjv.netproxy.feature.routing.model.SimpleRule
import com.fanjv.netproxy.feature.routing.model.policyRule
import com.fanjv.netproxy.feature.routing.model.simpleRule
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Test

class LocalRuleSetTest {
    @Test fun targetsAreOnlyTheThreeExistingLocalFiles() {
        assertEquals(listOf("singbox/rules/local/proxy.json", "singbox/rules/local/direct.json", "singbox/rules/local/block.json"),
            LocalRuleSet.shortcuts.map { it.target })
        assertEquals("singbox/route", LocalRuleSet.Policy.target)
    }

    @Test fun groupRulesKeepNativeGuardsShortcutsAndBuiltInPriority() {
        val original = LocalRuleDocument.parse(defaultRoute, nativeRoute = true)
        val first = RuleField.DomainSuffix.policyRule("example.com", "AI")
        val second = RuleField.Port.policyRule("8443", "Auto")
        val edited = original.replace(null, first).replace(null, second)
        assertEquals(listOf(5, 6), edited.editableIndices)
        assertEquals(original.rules.take(5), edited.rules.take(5))
        assertEquals(original.rules.drop(5), edited.rules.drop(7))
        assertEquals("AI", edited.rules[5].policyRule()?.first)
        assertEquals(edited, LocalRuleDocument.parse(edited.content(), nativeRoute = true))
        assertEquals(original, edited.remove(6).remove(5))
    }

    @Test fun customRouteWithoutSafeInsertionPointIsNotModified() {
        val document = LocalRuleDocument.parse(
            """{"route":{"rules":[{"ip_is_private":true,"outbound":"direct"}]}}""", nativeRoute = true,
        )
        assertNull(document.policyStart)
        assertThrows(IllegalArgumentException::class.java) {
            document.replace(null, RuleField.Domain.policyRule("example.com", "AI"))
        }
        val lateMode = defaultRoute.replace(
            """{"clash_mode":"Rule","action":"route","outbound":"Proxy"}""",
            """{"clash_mode":["Proxy","Direct"],"action":"route","outbound":"Proxy"}""",
        )
        assertNull(LocalRuleDocument.parse(lateMode, nativeRoute = true).policyStart)
    }

    @Test fun simpleRulesAcceptScalarAndArrayWithoutFlatteningAdvancedConditions() {
        for (source in listOf("""{"domain":"example.com"}""", """{"type":"default","domain":["example.com"]}""")) {
            assertEquals(listOf("example.com"), Json.parseToJsonElement(source).simpleRule()!!.values)
        }
        assertEquals(RuleField.Port, Json.parseToJsonElement("""{"port":443}""").simpleRule()!!.field)
        for (source in listOf(
            """{"domain":"example.com","port":443}""",
            """{"domain":"example.com","invert":true}""",
            """{"domain":"example.com","domain_match_strategy":"prefer_ip"}""",
            """{"type":"logical","mode":"or","rules":[{"domain":"example.com"}]}""",
            """{"port":"443"}""", """{"domain":[]}""", """{"domain":[null]}""", "{}",
            """{"domain_regex":"first\nsecond"}""",
        )) assertNull(source, Json.parseToJsonElement(source).simpleRule())
    }

    @Test fun editsPreserveVersionAdvancedRulesAndRootFieldsInOrder() {
        val original = LocalRuleDocument.parse("""{"version":5,"extra":{"kept":true},"rules":[
            {"domain":"example.com","port":443},
            {"type":"logical","mode":"and","rules":[{"ip_cidr":"192.0.2.0/24"},{"network":"tcp"}]},
            {"domain_suffix":"old.example"}]}""")
        val edited = original.replace(2, RuleField.Domain.rule("new.example"))
            .replace(null, RuleField.Port.rule("443\n8443"))
        assertEquals(original.rules.take(2), edited.rules.take(2))
        assertEquals(original.root["extra"], edited.root["extra"])
        assertEquals(JsonPrimitive(5), edited.root["version"])
        assertEquals(listOf("new.example"), edited.rules[2].simpleRule()!!.values)
        assertEquals(original.root["extra"], LocalRuleDocument.parse(edited.content()).root["extra"])
        val removed = edited.remove(2)
        assertEquals(3, removed.rules.size)
        assertEquals(edited.rules[3], removed.rules[2])
        assertEquals(3, original.rules.size)
    }

    @Test fun inputIsLineBasedAndPortsAreJsonNumbers() {
        val regex = "^a{1,3}\\.example$\n^b\\.example$"
        assertEquals(listOf("^a{1,3}\\.example$", "^b\\.example$"), RuleField.DomainRegex.values(regex))
        assertEquals(listOf("example.com", "second.example"), RuleField.DomainSuffix.values(" example.com\r\n\nsecond.example\nexample.com "))
        assertEquals(JsonArray(listOf(JsonPrimitive(443), JsonPrimitive(8443))), RuleField.Port.rule("443\n8443")["port"])
    }

    @Test fun ipAndPackageRulesRoundTripWithoutChangingTheirMatchFields() {
        for ((field, input) in listOf(
            RuleField.IpCidr to "192.168.10.0/24\n2001:db8::/32",
            RuleField.PackageName to "com.android.chrome\nandroid\ncom.example.app_2",
        )) {
            assertNull(field.inputError(input))
            val rule = field.rule(input)
            assertEquals(setOf(field.key), rule.keys)
            assertEquals(SimpleRule(field, input.lines()), rule.simpleRule())
            val document = LocalRuleDocument.parse("""{"version":1,"rules":[]}""")
                .replace(null, rule)
            assertEquals(document, LocalRuleDocument.parse(document.content()))
        }
        for (input in listOf("0:com.android.chrome", "com.example app", "https://example.com", "com..example", "com.2example", ".com.example")) {
            assertEquals(RuleInputError.PackageName, RuleField.PackageName.inputError(input))
        }
        assertEquals(RuleInputError.Empty, RuleField.PackageName.inputError(""))
        assertThrows(IllegalArgumentException::class.java) { RuleField.PackageName.rule("0:com.android.chrome") }
    }

    @Test fun invalidInputCannotBecomeAnUnconditionalRule() {
        assertEquals(RuleInputError.Empty, RuleField.DomainSuffix.inputError("\n "))
        assertEquals(RuleInputError.Domain, RuleField.Domain.inputError("https://example.com/path"))
        assertEquals(RuleInputError.Port, RuleField.Port.inputError("0\n443"))
        assertEquals(RuleInputError.Port, RuleField.Port.inputError("65536"))
        for (input in listOf("9000:8000", "1:65536", "8000-9000", "1:2:3", ":443")) {
            assertEquals(RuleInputError.PortRange, RuleField.PortRange.inputError(input))
        }
        assertNull(RuleField.PortRange.inputError("1:65535"))
        assertThrows(IllegalArgumentException::class.java) { RuleField.Domain.rule("") }
        for (source in listOf("{}", "null", "[]", """{"version":1,"rules":null}""", """{"version":99,"rules":[]}""")) {
            assertTrue(runCatching { LocalRuleDocument.parse(source) }.isFailure)
        }
    }
}

internal val defaultRoute = """
    {"route":{"final":"Proxy","find_process":true,"rules":[
      {"clash_mode":"Direct","action":"route","outbound":"direct"},
      {"action":"sniff"},
      {"type":"logical","mode":"or","rules":[{"port":53},{"protocol":"dns"}],"action":"hijack-dns"},
      {"clash_mode":"Proxy","action":"route","outbound":"Proxy"},
      {"type":"logical","mode":"and","rules":[{"clash_mode":"RuleAllowAds","invert":true},
         {"rule_set":["block","Ads_AWAvenue","geosite/category-ads-all"]}],"action":"reject"},
      {"rule_set":"geosite/category-ai-!cn","action":"route","outbound":"AI"},
      {"clash_mode":"Rule","action":"route","outbound":"Proxy"}
    ]}}
""".trimIndent()
