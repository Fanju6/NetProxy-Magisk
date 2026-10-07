package com.fanjv.netproxy.feature.inbound.data

import com.fanjv.netproxy.core.command.NetProxyCtlException
import com.fanjv.netproxy.core.module.ServiceRepository
import com.fanjv.netproxy.core.module.ServiceStatusSnapshot
import com.fanjv.netproxy.feature.settings.data.ConfigRepository
import com.fanjv.netproxy.feature.settings.model.ConfigSnapshot
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import java.net.NetworkInterface
import java.util.Collections

internal data class InboundSnapshot(
    val backend: String,
    val partitions: Map<String, ConfigSnapshot>,
    val status: ServiceStatusSnapshot?
)

internal data class InboundChoices(val interfaces: List<String>, val ruleSets: List<String>)
internal class InboundSwitchConfirmationRequired : IllegalStateException()

internal fun ServiceStatusSnapshot.mayBeRunning(): Boolean =
    state !in setOf("stopped", "failed") || pid != null
internal fun ServiceStatusSnapshot.requiresBackendSwitch(backend: String): Boolean =
    mayBeRunning() && activeBackend != backend

internal class InboundRepository(
    private val config: ConfigRepository,
    private val service: ServiceRepository
) {
    suspend fun load(): InboundSnapshot {
        val partitions = listOf("backend", "ebpf", "tun").associateWith {
            config.readSnapshot("inbound/$it")
        }
        val backend = inboundJson.parseToJsonElement(partitions.getValue("backend").content)
            .jsonObject.textAt("backend")
        check(backend in setOf("ebpf", "tun"))
        return InboundSnapshot(backend, partitions, service.status())
    }

    suspend fun choices(): InboundChoices {
        val main = inboundJson.parseToJsonElement(config.read("singbox/config.json")).jsonObject
        val ruleSets = (main.objectAt("route")["rule_set"] as? JsonArray).orEmpty()
            .flatMap { (it as? JsonObject)?.listAt("tag").orEmpty() }.filter(String::isNotEmpty).distinct()
        val interfaces = withContext(Dispatchers.IO) {
            NetworkInterface.getNetworkInterfaces()?.let { Collections.list(it).map { network -> network.name }.sorted() }.orEmpty()
        }
        return InboundChoices(interfaces, ruleSets)
    }

    suspend fun requiresSwitchConfirmation(target: String, content: String): Boolean {
        if (target !in setOf("inbound", "inbound/backend")) return false
        val backend = inboundJson.parseToJsonElement(content).jsonObject.textAt("backend")
        return service.status().requiresBackendSwitch(backend)
    }

    suspend fun apply(target: String, content: String, revision: String, confirmBackendSwitch: Boolean = false): String {
        require(revision.isNotBlank())
        val before = service.status()
        val requested = if (target in setOf("inbound", "inbound/backend")) {
            inboundJson.parseToJsonElement(content).jsonObject.textAt("backend")
        } else null
        if (requested != null && before.requiresBackendSwitch(requested) && !confirmBackendSwitch) {
            throw InboundSwitchConfirmationRequired()
        }
        val result = config.apply(target, content, revision)
        val after = service.status()
        val expected = requested ?: before.configuredBackend
        val selectedPartition = target == "inbound/$expected"
        if ((requested != null && after.configuredBackend != requested) ||
            ((requested != null || selectedPartition) && before.mayBeRunning() &&
                (after.state != "ready" || after.activeBackend != expected))
        ) {
            throw NetProxyCtlException("inbound.not_confirmed", "入站应用结果尚未确认，请重新加载")
        }
        return result
    }

    suspend fun status() = service.status()
    suspend fun restart() = service.action("restart")
    suspend fun diagnose() = config.ebpfStatus()
}
