package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit

import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test

class NodeEditDraftTest {
    private val content = """{
        "metadata":{"label":"untouched"},
        "outbounds":[{
            "type":"hysteria2","tag":"sample","server":"example.invalid","server_port":443,
            "password":"fixture-only","server_ports":"443:444","hop_interval":"1m",
            "heartbeat":"1h","up_mbps":100,"down_mbps":200,
            "obfs":{"type":"salamander","password":"fixture-only","extension":[1,2]},
            "tls":{"enabled":true,"insecure":false,"disable_sni":false,"alpn":"h3",
                "utls":{"enabled":false,"fingerprint":"chrome","extension":"keep"},
                "reality":{"enabled":false,"public_key":"fixture-only","short_id":"00"},
                "ech":{"enabled":true,"config":["first","second\nthird"],"config_path":"/fixture/ech.pem"},
                "certificate_public_key_sha256":["fixture-only"],"extension":{"nested":null}},
            "transport":{"type":"ws","headers":{"hOsT":["example.invalid"],"Other":["first","second"]},"max_early_data":2048},
            "multiplex":{"enabled":true,"padding":false},"extension":{"nested":[1,false,null]}
        },{"type":"direct","tag":"other"}]
    }"""

    @Test fun unchangedFormPreservesAllOriginalJsonAndListableShapes() {
        val draft = NodeEditDraft.parse(content)
        assertEquals(draft.original, draft.toJson())
        assertEquals("1m", draft.fields.hopInterval)
        assertEquals("1h", draft.fields.heartbeat)
        assertEquals("none", draft.fields.fingerprint)
        assertEquals("first\nsecond\nthird", draft.fields.echConfig)
    }

    @Test fun renamingOnlyChangesTagIncludingWrappedMetadataAndOtherOutbounds() {
        val draft = NodeEditDraft.parse(content)
        val result = draft.copy(fields = draft.fields.copy(tag = "renamed")).toJson()
        val expected = content.replace("\"tag\":\"sample\"", "\"tag\":\"renamed\"")
        assertEquals(NodeEditDraft.parse(expected).original, result)
    }

    @Test fun absentDefaultFieldsAreNotWrittenAndRevertedEditsDoNotNormalizeJson() {
        for (type in listOf("vless", "vmess", "shadowsocks", "trojan", "hysteria2", "tuic", "anytls")) {
            val draft = NodeEditDraft.parse("""{"type":"$type","tag":"sample","server":"example.invalid","server_port":443}""")
            assertEquals(draft.original, draft.toJson())
            val reverted = draft.copy(fields = draft.fields.copy(tag = "new")).copy(fields = draft.fields)
            assertEquals(draft.original, reverted.toJson())
        }
    }

    @Test fun durationEditsKeepNativeUnitsAndDoNotTouchOtherFields() {
        val draft = NodeEditDraft.parse("""{"type":"tuic","tag":"sample","hop_interval":"1m","heartbeat":"1h","extension":false}""")
        for (duration in listOf("1m", "1h", "1500ms", "1m30s", "0.5s")) {
            val edited = draft.copy(fields = draft.fields.copy(heartbeat = duration)).toJson()
            assertEquals(JsonPrimitive(duration), edited["heartbeat"])
            assertEquals(draft.original - "heartbeat", edited - "heartbeat")
        }
    }

    @Test fun editingTlsKeepsDisabledUtlsMultiEchAndNestedNativeFields() {
        val draft = NodeEditDraft.parse(content)
        val before = draft.original["outbounds"]!!.let { (it as kotlinx.serialization.json.JsonArray).first().jsonObject }
        val result = draft.copy(fields = draft.fields.copy(serverName = "changed.invalid")).toJson()
        val after = (result["outbounds"] as kotlinx.serialization.json.JsonArray).first().jsonObject
        assertEquals(before - "tls", after - "tls")
        assertEquals(before["tls"]!!.jsonObject - "server_name", after["tls"]!!.jsonObject - "server_name")
        assertEquals(JsonPrimitive("changed.invalid"), after["tls"]!!.jsonObject["server_name"])
    }

    @Test fun disablingTlsAndEchOnlyChangesEnabledFlags() {
        val draft = NodeEditDraft.parse("""{"tls":{"enabled":true,"utls":{"enabled":false,"fingerprint":"chrome"},"ech":{"enabled":true,"config":["first","second"]}}}""")
        val result = draft.copy(fields = draft.fields.copy(tlsEnabled = false, echEnabled = false)).toJson()
        val tls = result["tls"]!!.jsonObject
        assertEquals(JsonPrimitive(false), tls["enabled"])
        assertEquals(draft.original["tls"]!!.jsonObject["utls"], tls["utls"])
        assertEquals(draft.original["tls"]!!.jsonObject["ech"]!!.jsonObject - "enabled", tls["ech"]!!.jsonObject - "enabled")
    }

    @Test fun editingHostPreservesOtherHeadersAndTransportFields() {
        val draft = NodeEditDraft.parse("""{"transport":{"type":"ws","headers":{"hOsT":"before.invalid","Other":["one","two"]},"max_early_data":2048}}""")
        val result = draft.copy(fields = draft.fields.copy(host = "after.invalid")).toJson()
        val headers = result["transport"]!!.jsonObject["headers"]!!.jsonObject
        assertEquals(JsonPrimitive("after.invalid"), headers["hOsT"])
        assertEquals(draft.original["transport"]!!.jsonObject["headers"]!!.jsonObject["Other"], headers["Other"])
        assertEquals(draft.original["transport"]!!.jsonObject - "headers", result["transport"]!!.jsonObject - "headers")
    }

    @Test fun reinitializingRetainedViewModelDoesNotDiscardDraft() {
        val model = NodeEditViewModel()
        model.initialize(content)
        model.update { it.copy(tag = "unsaved", password = "anonymous-new-value", heartbeat = "1500ms") }
        val retained = model.draft.value
        model.initialize(content)
        assertEquals(retained, model.draft.value)
        assertEquals("unsaved", model.draft.value!!.fields.tag)
    }

    @Test fun switchingProtocolDropsOldOwnedFieldsAndInitializesNewFields() {
        val draft = NodeEditDraft.parse("""{"type":"vless","tag":"sample","server":"example.invalid","server_port":443,"uuid":"fixture-only","flow":"xtls-rprx-vision","packet_encoding":"xudp","tls":{"enabled":false},"multiplex":{"enabled":true},"connect_timeout":"1m","extension":{"keep":true}}""")
        val result = draft.copy(fields = draft.fields.copy(type = "shadowsocks", password = "fixture-new")).toJson()
        assertEquals(JsonPrimitive("shadowsocks"), result["type"])
        assertEquals(JsonPrimitive("aes-128-gcm"), result["method"])
        assertEquals(JsonPrimitive("fixture-new"), result["password"])
        for (key in listOf("uuid", "flow", "packet_encoding", "tls")) assertFalse(result.containsKey(key))
        for (key in listOf("tag", "server", "server_port", "multiplex", "connect_timeout", "extension")) {
            assertEquals(draft.original[key], result[key])
        }
    }

    @Test fun switchingProtocolsPreservesSharedNativeFieldsInsteadOfNormalizingThem() {
        val draft = NodeEditDraft.parse("""{"type":"vmess","uuid":"fixture-only","security":"auto","global_padding":false,"alter_id":0,"network":"tcp","tls":{"enabled":false},"multiplex":{"enabled":false},"transport":{"type":"ws","headers":{"Other":"keep"}},"extension":null}""")
        val result = draft.copy(fields = draft.fields.copy(type = "vless")).toJson()
        for (key in listOf("security", "global_padding", "alter_id")) assertFalse(result.containsKey(key))
        for (key in listOf("uuid", "network", "tls", "multiplex", "transport", "extension")) assertEquals(draft.original[key], result[key])
        assertFalse(result.containsKey("flow"))
    }

    @Test fun switchingHttpToWsMovesHostEvenWhenItsInputWasNotChanged() {
        val draft = NodeEditDraft.parse("""{"metadata":{"keep":true},"outbounds":[{"type":"vless","transport":{"type":"http","host":["example.invalid"],"path":"/path","method":"PUT","idle_timeout":"1m","headers":{"Other":["one","two"]},"extension":true}}]}""")
        val result = draft.copy(fields = draft.fields.copy(transportType = "ws")).toJson()
        val node = (result["outbounds"] as kotlinx.serialization.json.JsonArray).first().jsonObject
        val transport = node["transport"]!!.jsonObject
        assertEquals(JsonPrimitive("ws"), transport["type"])
        assertEquals(JsonPrimitive("example.invalid"), transport["headers"]!!.jsonObject["Host"])
        assertEquals(JsonPrimitive("/path"), transport["path"])
        assertEquals(JsonPrimitive(true), transport["extension"])
        for (key in listOf("host", "method", "idle_timeout")) assertFalse(transport.containsKey(key))
        assertEquals(draft.original["metadata"], result["metadata"])
    }

    @Test fun switchingWsToGrpcDropsWsFieldsAndWritesServiceName() {
        val draft = NodeEditDraft.parse("""{"transport":{"type":"ws","path":"/path","headers":{"Host":"example.invalid"},"max_early_data":2048,"early_data_header_name":"X-Test","extension":"keep"}}""")
        val result = draft.copy(fields = draft.fields.copy(transportType = "grpc", serviceName = "sample")).toJson()
        val transport = result["transport"]!!.jsonObject
        assertEquals(setOf("type", "service_name", "extension"), transport.keys)
        assertEquals(JsonPrimitive("sample"), transport["service_name"])
        assertEquals(JsonPrimitive("keep"), transport["extension"])
    }

    @Test fun switchingObfsDropsOnlyOldSpecificFieldsAndKeepsPasswordAndUnknownFields() {
        val draft = NodeEditDraft.parse("""{"type":"hysteria2","obfs":{"type":"gecko","password":"fixture-only","min_packet_size":64,"max_packet_size":1500,"extension":"keep"}}""")
        val result = draft.copy(fields = draft.fields.copy(obfsType = "salamander")).toJson()
        val obfs = result["obfs"]!!.jsonObject
        assertEquals(setOf("type", "password", "extension"), obfs.keys)
        assertEquals(JsonPrimitive("salamander"), obfs["type"])
        assertEquals(draft.original["obfs"]!!.jsonObject["password"], obfs["password"])
        assertEquals(JsonPrimitive("keep"), obfs["extension"])
    }
}
