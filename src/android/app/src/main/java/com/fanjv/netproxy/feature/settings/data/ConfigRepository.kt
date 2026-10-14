package com.fanjv.netproxy.feature.settings.data

import com.fanjv.netproxy.core.api.ModuleApiClient
import com.fanjv.netproxy.feature.settings.model.ManagedConfigDocument
import com.fanjv.netproxy.feature.settings.model.ConfigSnapshot
import com.fanjv.netproxy.feature.settings.model.ModuleAutoStartConfig
import com.fanjv.netproxy.feature.settings.model.ModuleWifiConfig
import com.fanjv.netproxy.feature.settings.model.WifiPolicySettings
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** 模块与 sing-box 配置的事务读取、校验和写入入口。 */
internal class ConfigRepository(
    private val client: ModuleApiClient,
    private val awaitPendingWrites: suspend (String) -> Unit = {}
) {
    private val moduleJson = Json(client.json) {
        ignoreUnknownKeys = false
        encodeDefaults = true
    }

    suspend fun listDocuments(): List<ManagedConfigDocument> =
        client.read("/configs")

    suspend fun read(target: String): String =
        readSnapshot(target).content

    suspend fun readSnapshot(target: String): ConfigSnapshot {
        awaitPendingWrites(target)
        return client.read("/config", mapOf("target" to target))
    }

    suspend fun readWifi(): ModuleWifiConfig = readSnapshot("module/wifi").let {
        val content = moduleJson.parseToJsonElement(it.content).jsonObject
        (content["wifi"] as? JsonObject)?.let { wifi ->
            wifi.requireBoolean("enabled")
            wifi.requireBoolean("proxy_on_non_wifi")
        }
        moduleJson.decodeFromJsonElement<ModuleWifiConfig>(content).copy(revision = it.revision)
    }

    suspend fun readAutoStart(): ModuleAutoStartConfig = readSnapshot("module/auto_start").let {
        val content = moduleJson.parseToJsonElement(it.content).jsonObject
        content.requireBoolean("auto_start")
        moduleJson.decodeFromJsonElement<ModuleAutoStartConfig>(content).copy(revision = it.revision)
    }

    private fun JsonObject.requireBoolean(key: String) {
        val value = get(key) ?: return
        // kotlinx.serialization 也接受引号布尔值，必须在类型解码前拒绝它们。
        require(value is JsonPrimitive && !value.isString && value.booleanOrNull != null) {
            "$key 必须为 JSON 布尔值"
        }
    }

    suspend fun applyWifi(wifi: WifiPolicySettings, revision: String): String =
        apply("module/wifi", moduleJson.encodeToString(ModuleWifiConfig(wifi)), revision)

    suspend fun applyAutoStart(enabled: Boolean, revision: String): String =
        apply("module/auto_start", moduleJson.encodeToString(ModuleAutoStartConfig(enabled)), revision)

    suspend fun apply(target: String, content: String, revision: String): String =
        client.put("/config", buildJsonObject { put("content", content); put("revision", revision) },
            mapOf("target" to target)).data.jsonObject["revision"]?.jsonPrimitive?.content
            ?: error("模块没有返回配置版本")

    suspend fun check() {
        client.post("/service/check")
    }

    suspend fun validate(target: String, content: String, revision: String) {
        client.post("/config/validate", buildJsonObject { put("content", content); put("revision", revision) },
            mutation = false, query = mapOf("target" to target))
    }

    suspend fun ebpfStatus(mode: String = "configured"): String =
        client.post("/diagnostics/ebpf", buildJsonObject { put("mode", mode) }).data.jsonObject["content"]
            ?.jsonPrimitive?.content
            ?: error("模块没有返回 eBPF 诊断结果")

    suspend fun savedWifiNetworks(): List<String> =
        client.json.decodeFromJsonElement(client.get("/network/wifi-list").data.jsonObject.getValue("ssids"))
}
