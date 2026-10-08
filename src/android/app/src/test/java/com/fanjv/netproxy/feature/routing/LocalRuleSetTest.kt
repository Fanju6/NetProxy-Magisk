package com.fanjv.netproxy.feature.routing

import com.fanjv.netproxy.feature.routing.model.LocalRuleDocument
import com.fanjv.netproxy.feature.routing.model.LocalRuleSet
import com.fanjv.netproxy.feature.routing.model.RuleField
import com.fanjv.netproxy.feature.routing.model.RuleInputError
import com.fanjv.netproxy.feature.routing.model.simpleRule
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.*
import org.junit.Test

class LocalRuleSetTest {
    @Test fun targetsAreOnlyTheThreeExistingLocalFiles() {
        assertEquals(listOf("singbox/rules/local/proxy.json", "singbox/rules/local/direct.json", "singbox/rules/local/block.json"),
            LocalRuleSet.entries.map { it.target })
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
