package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.catalog.presentation.nodes.CatalogNodesViewModel
import com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components.ActionButtons
import com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components.ServerConfigSection
import com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components.TlsConfigSection
import com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components.TransportSection
import com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components.ValidationPanel
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@OptIn(ExperimentalLayoutApi::class)
/** sing-box 节点编辑页：按协议编辑出站配置。 */
@Composable
internal fun SingBoxNodeEditScreen(
    viewModel: CatalogNodesViewModel,
    nodeRef: String,
    onBack: () -> Unit
) {
    val focusManager = LocalFocusManager.current
    val backdrop = rememberBlurBackdrop()
    val blurActive = backdrop != null
    val barColor = if (blurActive) Color.Transparent else colorScheme.surface
    val scrollBehavior = MiuixScrollBehavior()
    val snackbarHostState = rememberAppSnackbarHostState()
    var notice by remember { mutableStateOf("") }
    var noticeId by remember { mutableStateOf(0L) }

    ValidationPanel(
        eventId = noticeId,
        message = notice,
        isError = true,
        hostState = snackbarHostState,
        onConsumed = { notice = "" }
    )

    // 在 composable 层取字符串资源以供回调使用
    val nodeTagEmpty = stringResource(R.string.node_tag_empty)
    val serverAddressEmpty = stringResource(R.string.server_address_empty)
    val serverPortEmpty = stringResource(R.string.server_port_empty)
    val saveFailedCheckPermission = stringResource(R.string.save_failed_check_permission)
    val alterIdLabel = stringResource(R.string.alterid_label)
    val udpRelayModeTitle = stringResource(R.string.udp_relay_mode)
    val alpnLabel = stringResource(R.string.alpn_label)
    val utlsFingerprint = stringResource(R.string.utls_fingerprint)
    val realityPublicKeyLabel = stringResource(R.string.reality_public_key)
    val realityShortIdLabel = stringResource(R.string.reality_short_id)
    val echConfigListLabel = stringResource(R.string.ech_config_list)
    val echDnsServerNameLabel = stringResource(R.string.ech_dns_server_name)

    // 常规状态
    var tag by remember { mutableStateOf("") }
    var type by remember { mutableStateOf("vless") }
    var server by remember { mutableStateOf("") }
    var serverPort by remember { mutableStateOf("") }

    // VLESS / VMess 的 UUID
    var uuid by remember { mutableStateOf("") }
    var flow by remember { mutableStateOf("none") } // VLESS

    // VMess 专用
    var security by remember { mutableStateOf("auto") }
    var alterId by remember { mutableStateOf("") }

    // Shadowsocks 专用
    var method by remember { mutableStateOf("aes-128-gcm") }
    var password by remember { mutableStateOf("") }
    var plugin by remember { mutableStateOf("") }
    var pluginOpts by remember { mutableStateOf("") }

    // Hysteria2 专用
    var upMbps by remember { mutableStateOf("") }
    var downMbps by remember { mutableStateOf("") }
    var obfsType by remember { mutableStateOf("none") }
    var obfsPassword by remember { mutableStateOf("") }
    var serverPorts by remember { mutableStateOf("") }
    var hopInterval by remember { mutableStateOf("") }

    // TUIC 专用
    var congestionControl by remember { mutableStateOf("cubic") }
    var udpRelayMode by remember { mutableStateOf("quic") }
    var udpOverStream by remember { mutableStateOf(false) }
    var zeroRttHandshake by remember { mutableStateOf(false) }
    var heartbeat by remember { mutableStateOf("") }

    // 传输层状态
    var transportType by remember { mutableStateOf("none") }
    var path by remember { mutableStateOf("") }
    var host by remember { mutableStateOf("") }
    var serviceName by remember { mutableStateOf("") }

    // TLS 状态
    var tlsEnabled by remember { mutableStateOf(false) }
    var serverName by remember { mutableStateOf("") }
    var insecure by remember { mutableStateOf(false) }
    var disableSni by remember { mutableStateOf(false) }
    var alpn by remember { mutableStateOf("") }
    var fingerprint by remember { mutableStateOf("none") }
    var realityEnabled by remember { mutableStateOf(false) }
    var realityPublicKey by remember { mutableStateOf("") }
    var realityShortId by remember { mutableStateOf("") }
    var echEnabled by remember { mutableStateOf(false) }
    var echConfig by remember { mutableStateOf("") }
    var echQueryServerName by remember { mutableStateOf("") }

    // 保留未映射的原始 JSON
    val rawJsonMap = remember { mutableStateMapOf<String, JsonElement>() }
    var isWrappedOriginal by remember { mutableStateOf(true) }

    // 保留原始嵌套结构
    var originalTlsJson by remember { mutableStateOf<JsonObject?>(null) }
    var originalTransportJson by remember { mutableStateOf<JsonObject?>(null) }
    var originalUtlsJson by remember { mutableStateOf<JsonObject?>(null) }
    var originalRealityJson by remember { mutableStateOf<JsonObject?>(null) }
    var originalEchJson by remember { mutableStateOf<JsonObject?>(null) }
    var originalObfsJson by remember { mutableStateOf<JsonObject?>(null) }

    // 加载配置内容
    LaunchedEffect(nodeRef) {
        val jsonString = runCatching {
            viewModel.loadNodeConfigContent(nodeRef)
        }.getOrNull()
        if (jsonString != null) {
            val root = try {
                Json.parseToJsonElement(jsonString).jsonObject
            } catch (_: Exception) {
                null
            }
            if (root != null) {
                isWrappedOriginal = root.containsKey("outbounds")
                val outbound = if (isWrappedOriginal) {
                    root["outbounds"]?.jsonArray?.firstOrNull()?.jsonObject
                } else {
                    root
                }

                if (outbound != null) {
                    tag = outbound["tag"]?.jsonPrimitive?.contentOrNull ?: ""
                    type = outbound["type"]?.jsonPrimitive?.contentOrNull ?: "vless"
                    server = outbound["server"]?.jsonPrimitive?.contentOrNull ?: ""
                    serverPort = outbound["server_port"]?.jsonPrimitive?.contentOrNull
                        ?: outbound["server_port"]?.jsonPrimitive?.intOrNull?.toString()
                                ?: ""

                    uuid = outbound["uuid"]?.jsonPrimitive?.contentOrNull ?: ""
                    flow = outbound["flow"]?.jsonPrimitive?.contentOrNull ?: "none"
                    security = outbound["security"]?.jsonPrimitive?.contentOrNull ?: "auto"
                    alterId = outbound["alter_id"]?.jsonPrimitive?.intOrNull?.toString() ?: ""
                    password = outbound["password"]?.jsonPrimitive?.contentOrNull ?: ""
                    method = outbound["method"]?.jsonPrimitive?.contentOrNull ?: "aes-128-gcm"
                    plugin = outbound["plugin"]?.jsonPrimitive?.contentOrNull ?: ""
                    pluginOpts = outbound["plugin_opts"]?.jsonPrimitive?.contentOrNull ?: ""

                    // Hysteria2 配置
                    upMbps = outbound["up_mbps"]?.jsonPrimitive?.intOrNull?.toString() ?: ""
                    downMbps = outbound["down_mbps"]?.jsonPrimitive?.intOrNull?.toString() ?: ""
                    val obfsObj = outbound["obfs"]?.jsonObject
                    if (obfsObj != null) {
                        originalObfsJson = obfsObj
                        obfsType = obfsObj["type"]?.jsonPrimitive?.contentOrNull ?: "none"
                        obfsPassword = obfsObj["password"]?.jsonPrimitive?.contentOrNull ?: ""
                    }
                    val serverPortsArray = outbound["server_ports"]?.jsonArray
                    if (serverPortsArray != null) {
                        serverPorts = serverPortsArray.mapNotNull { it.jsonPrimitive.contentOrNull }
                            .joinToString(",")
                    }
                    hopInterval = outbound["hop_interval"]?.jsonPrimitive?.contentOrNull
                        ?.removeSuffix("s") ?: ""

                    // TUIC 配置
                    congestionControl =
                        outbound["congestion_control"]?.jsonPrimitive?.contentOrNull ?: "cubic"
                    udpRelayMode =
                        outbound["udp_relay_mode"]?.jsonPrimitive?.contentOrNull ?: "quic"
                    udpOverStream =
                        outbound["udp_over_stream"]?.jsonPrimitive?.booleanOrNull ?: false
                    zeroRttHandshake =
                        outbound["zero_rtt_handshake"]?.jsonPrimitive?.booleanOrNull ?: false
                    heartbeat = outbound["heartbeat"]?.jsonPrimitive?.contentOrNull
                        ?.removeSuffix("s") ?: ""

                    // 传输层
                    val transport = outbound["transport"]?.jsonObject
                    if (transport != null) {
                        originalTransportJson = transport
                        transportType = transport["type"]?.jsonPrimitive?.contentOrNull ?: "none"
                        path = transport["path"]?.jsonPrimitive?.contentOrNull ?: ""
                        val hostArray = transport["host"]?.jsonArray
                        if (hostArray != null) {
                            host = hostArray.mapNotNull { it.jsonPrimitive.contentOrNull }
                                .joinToString(",")
                        } else {
                            val headers = transport["headers"]?.jsonObject
                            if (headers != null) {
                                host = headers["Host"]?.jsonPrimitive?.contentOrNull
                                    ?: headers["host"]?.jsonPrimitive?.contentOrNull
                                            ?: ""
                            }
                        }
                        serviceName = transport["service_name"]?.jsonPrimitive?.contentOrNull ?: ""
                    }

                    // TLS 配置
                    val tls = outbound["tls"]?.jsonObject
                    if (tls != null) {
                        originalTlsJson = tls
                        tlsEnabled = tls["enabled"]?.jsonPrimitive?.booleanOrNull ?: false
                        serverName = tls["server_name"]?.jsonPrimitive?.contentOrNull ?: ""
                        insecure = tls["insecure"]?.jsonPrimitive?.booleanOrNull ?: false
                        disableSni = tls["disable_sni"]?.jsonPrimitive?.booleanOrNull ?: false
                        val alpnArray = tls["alpn"]?.jsonArray
                        if (alpnArray != null) {
                            alpn = alpnArray.mapNotNull { it.jsonPrimitive.contentOrNull }
                                .joinToString(",")
                        }
                        val utls = tls["utls"]?.jsonObject
                        if (utls != null) {
                            originalUtlsJson = utls
                            fingerprint =
                                utls["fingerprint"]?.jsonPrimitive?.contentOrNull ?: "chrome"
                        } else {
                            fingerprint = "none"
                        }
                        val reality = tls["reality"]?.jsonObject
                        if (reality != null) {
                            originalRealityJson = reality
                            realityEnabled =
                                reality["enabled"]?.jsonPrimitive?.booleanOrNull ?: false
                            realityPublicKey =
                                reality["public_key"]?.jsonPrimitive?.contentOrNull ?: ""
                            realityShortId = reality["short_id"]?.jsonPrimitive?.contentOrNull ?: ""
                        }
                        val ech = tls["ech"]?.jsonObject
                        if (ech != null) {
                            originalEchJson = ech
                            echEnabled = ech["enabled"]?.jsonPrimitive?.booleanOrNull ?: false
                            val echConfigArray = ech["config"]?.jsonArray
                            if (echConfigArray != null) {
                                echConfig =
                                    echConfigArray.firstOrNull()?.jsonPrimitive?.contentOrNull ?: ""
                            }
                            echQueryServerName =
                                ech["query_server_name"]?.jsonPrimitive?.contentOrNull ?: ""
                        }
                    }

                    // 保留其他键
                    val handledKeys = setOf(
                        "tag",
                        "type",
                        "server",
                        "server_port",
                        "uuid",
                        "flow",
                        "security",
                        "alter_id",
                        "password",
                        "method",
                        "plugin",
                        "plugin_opts",
                        "up_mbps",
                        "down_mbps",
                        "obfs",
                        "server_ports",
                        "hop_interval",
                        "congestion_control",
                        "udp_relay_mode",
                        "udp_over_stream",
                        "zero_rtt_handshake",
                        "heartbeat",
                        "transport",
                        "tls"
                    )
                    outbound.forEach { (key, value) ->
                        if (!handledKeys.contains(key)) {
                            rawJsonMap[key] = value
                        }
                    }
                }
            }
        }
    }

    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbarHostState) },
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    color = barColor,
                    title = stringResource(R.string.edit_node),
                    scrollBehavior = scrollBehavior,
                    navigationIcon = {
                        IconButton(onClick = onBack) {
                            Icon(
                                imageVector = Icons.AutoMirrored.Rounded.ArrowBack,
                                contentDescription = stringResource(R.string.back),
                                tint = colorScheme.onBackground
                            )
                        }
                    },
                    actions = {
                        ActionButtons(onSave = {
                            if (tag.isBlank()) {
                                notice = nodeTagEmpty
                                noticeId++
                                return@ActionButtons
                            }
                            if (server.isBlank()) {
                                notice = serverAddressEmpty
                                noticeId++
                                return@ActionButtons
                            }
                            if (serverPort.isBlank()) {
                                notice = serverPortEmpty
                                noticeId++
                                return@ActionButtons
                            }

                            val updatedOutbound = buildJsonObject {
                                put("type", JsonPrimitive(type))
                                put("tag", JsonPrimitive(tag))
                                put("server", JsonPrimitive(server))

                                val portVal = serverPort.trim().toIntOrNull()
                                if (portVal != null) {
                                    put("server_port", portVal)
                                } else {
                                    put("server_port", serverPort)
                                }

                                // 协议相关配置
                                when (type) {
                                    "vless" -> {
                                        put("uuid", JsonPrimitive(uuid))
                                        if (flow != "none") {
                                            put("flow", JsonPrimitive(flow))
                                        }
                                    }

                                    "vmess" -> {
                                        put("uuid", JsonPrimitive(uuid))
                                        put("security", JsonPrimitive(security))
                                        alterId.trim().toIntOrNull()?.let {
                                            put("alter_id", it)
                                        }
                                    }

                                    "shadowsocks" -> {
                                        put("method", JsonPrimitive(method))
                                        put("password", JsonPrimitive(password))
                                        if (plugin.isNotEmpty()) {
                                            put("plugin", JsonPrimitive(plugin))
                                        }
                                        if (pluginOpts.isNotEmpty()) {
                                            put("plugin_opts", JsonPrimitive(pluginOpts))
                                        }
                                    }

                                    "trojan", "anytls" -> {
                                        put("password", JsonPrimitive(password))
                                    }

                                    "hysteria2" -> {
                                        put("password", JsonPrimitive(password))
                                        upMbps.trim().toIntOrNull()?.let { put("up_mbps", it) }
                                        downMbps.trim().toIntOrNull()
                                            ?.let { put("down_mbps", it) }
                                        if (obfsType != "none" && obfsPassword.isNotEmpty()) {
                                            put("obfs", buildJsonObject {
                                                val origObfsMap =
                                                    originalObfsJson?.toMutableMap()
                                                        ?: mutableMapOf()
                                                origObfsMap["type"] = JsonPrimitive(obfsType)
                                                origObfsMap["password"] =
                                                    JsonPrimitive(obfsPassword)
                                                origObfsMap.forEach { (k, v) -> put(k, v) }
                                            })
                                        }
                                        if (serverPorts.isNotEmpty()) {
                                            val ports = serverPorts.split(",").map { it.trim() }
                                                .filter { it.isNotEmpty() }
                                            put(
                                                "server_ports",
                                                JsonArray(ports.map { JsonPrimitive(it) })
                                            )
                                        }
                                        if (hopInterval.isNotEmpty()) {
                                            put(
                                                "hop_interval",
                                                JsonPrimitive("${hopInterval}s")
                                            )
                                        }
                                    }

                                    "tuic" -> {
                                        if (uuid.isNotEmpty()) put("uuid", JsonPrimitive(uuid))
                                        put("password", JsonPrimitive(password))
                                        put(
                                            "congestion_control",
                                            JsonPrimitive(congestionControl)
                                        )
                                        put("udp_relay_mode", JsonPrimitive(udpRelayMode))
                                        put("udp_over_stream", JsonPrimitive(udpOverStream))
                                        put(
                                            "zero_rtt_handshake",
                                            JsonPrimitive(zeroRttHandshake)
                                        )
                                        if (heartbeat.isNotEmpty()) {
                                            put("heartbeat", JsonPrimitive("${heartbeat}s"))
                                        }
                                    }
                                }

                                // 传输层
                                if (transportType != "none") {
                                    put("transport", buildJsonObject {
                                        val origTransMap = if (originalTransportJson != null &&
                                            originalTransportJson?.get("type")?.jsonPrimitive?.contentOrNull == transportType
                                        ) {
                                            originalTransportJson!!.toMutableMap()
                                        } else {
                                            mutableMapOf()
                                        }

                                        origTransMap["type"] = JsonPrimitive(transportType)
                                        when (transportType) {
                                            "ws", "httpupgrade" -> {
                                                origTransMap["path"] =
                                                    JsonPrimitive(path.ifBlank { "/" })
                                                if (host.isNotEmpty()) {
                                                    val origHeaders = (origTransMap["headers"]
                                                        ?: originalTransportJson?.get("headers"))?.jsonObject?.toMutableMap()
                                                        ?: mutableMapOf()
                                                    val hostKey = origHeaders.keys.firstOrNull {
                                                        it.equals(
                                                            "host",
                                                            ignoreCase = true
                                                        )
                                                    } ?: "Host"
                                                    origHeaders[hostKey] = JsonPrimitive(host)
                                                    origTransMap["headers"] = buildJsonObject {
                                                        origHeaders.forEach { (k, v) ->
                                                            put(
                                                                k,
                                                                v
                                                            )
                                                        }
                                                    }
                                                } else {
                                                    val origHeaders = (origTransMap["headers"]
                                                        ?: originalTransportJson?.get("headers"))?.jsonObject?.toMutableMap()
                                                        ?: mutableMapOf()
                                                    val hostKey = origHeaders.keys.firstOrNull {
                                                        it.equals(
                                                            "host",
                                                            ignoreCase = true
                                                        )
                                                    }
                                                    if (hostKey != null) {
                                                        origHeaders.remove(hostKey)
                                                    }
                                                    if (origHeaders.isNotEmpty()) {
                                                        origTransMap["headers"] =
                                                            buildJsonObject {
                                                                origHeaders.forEach { (k, v) ->
                                                                    put(
                                                                        k,
                                                                        v
                                                                    )
                                                                }
                                                            }
                                                    } else {
                                                        origTransMap.remove("headers")
                                                    }
                                                }
                                            }

                                            "grpc" -> {
                                                origTransMap["service_name"] =
                                                    JsonPrimitive(serviceName)
                                            }

                                            "http", "h2" -> {
                                                if (path.isNotEmpty()) {
                                                    origTransMap["path"] = JsonPrimitive(path)
                                                } else {
                                                    origTransMap.remove("path")
                                                }
                                                if (host.isNotEmpty()) {
                                                    val hosts =
                                                        host.split(",").map { it.trim() }
                                                            .filter { it.isNotEmpty() }
                                                    origTransMap["host"] =
                                                        JsonArray(hosts.map { JsonPrimitive(it) })
                                                } else {
                                                    origTransMap.remove("host")
                                                }
                                            }
                                        }

                                        origTransMap.forEach { (key, value) ->
                                            put(key, value)
                                        }
                                    })
                                }

                                // TLS 配置
                                if (tlsEnabled) {
                                    put("tls", buildJsonObject {
                                        val origTlsMap =
                                            originalTlsJson?.toMutableMap() ?: mutableMapOf()

                                        origTlsMap["enabled"] = JsonPrimitive(true)
                                        if (serverName.isNotEmpty()) {
                                            origTlsMap["server_name"] =
                                                JsonPrimitive(serverName)
                                        } else {
                                            origTlsMap.remove("server_name")
                                        }

                                        if (insecure) {
                                            origTlsMap["insecure"] = JsonPrimitive(true)
                                        } else {
                                            origTlsMap.remove("insecure")
                                        }

                                        if (disableSni) {
                                            origTlsMap["disable_sni"] = JsonPrimitive(true)
                                        } else {
                                            origTlsMap.remove("disable_sni")
                                        }

                                        if (alpn.isNotEmpty()) {
                                            val alpns = alpn.split(",").map { it.trim() }
                                                .filter { it.isNotEmpty() }
                                            origTlsMap["alpn"] =
                                                JsonArray(alpns.map { JsonPrimitive(it) })
                                        } else {
                                            origTlsMap.remove("alpn")
                                        }

                                        if (fingerprint.isNotEmpty() && fingerprint != "none") {
                                            val origUtlsMap = originalUtlsJson?.toMutableMap()
                                                ?: mutableMapOf()
                                            origUtlsMap["enabled"] = JsonPrimitive(true)
                                            origUtlsMap["fingerprint"] =
                                                JsonPrimitive(fingerprint)
                                            origTlsMap["utls"] = buildJsonObject {
                                                origUtlsMap.forEach { (k, v) -> put(k, v) }
                                            }
                                        } else {
                                            origTlsMap.remove("utls")
                                        }

                                        if (realityEnabled) {
                                            val origRealityMap =
                                                originalRealityJson?.toMutableMap()
                                                    ?: mutableMapOf()
                                            origRealityMap["enabled"] = JsonPrimitive(true)
                                            origRealityMap["public_key"] =
                                                JsonPrimitive(realityPublicKey)
                                            if (realityShortId.isNotEmpty()) {
                                                origRealityMap["short_id"] =
                                                    JsonPrimitive(realityShortId)
                                            } else {
                                                origRealityMap.remove("short_id")
                                            }
                                            origTlsMap["reality"] = buildJsonObject {
                                                origRealityMap.forEach { (k, v) -> put(k, v) }
                                            }
                                        } else {
                                            origTlsMap.remove("reality")
                                        }

                                        if (echEnabled) {
                                            val origEchMap = originalEchJson?.toMutableMap()
                                                ?: mutableMapOf()
                                            origEchMap["enabled"] = JsonPrimitive(true)
                                            if (echConfig.isNotEmpty()) {
                                                origEchMap["config"] =
                                                    JsonArray(listOf(JsonPrimitive(echConfig)))
                                            } else {
                                                origEchMap.remove("config")
                                            }
                                            if (echQueryServerName.isNotEmpty()) {
                                                origEchMap["query_server_name"] =
                                                    JsonPrimitive(echQueryServerName)
                                            } else {
                                                origEchMap.remove("query_server_name")
                                            }
                                            origTlsMap["ech"] = buildJsonObject {
                                                origEchMap.forEach { (k, v) -> put(k, v) }
                                            }
                                        } else {
                                            origTlsMap.remove("ech")
                                        }

                                        origTlsMap.forEach { (key, value) ->
                                            put(key, value)
                                        }
                                    })
                                }

                                // 还原原始属性
                                rawJsonMap.forEach { (key, value) ->
                                    put(key, value)
                                }
                            }

                            val finalJson = if (isWrappedOriginal) {
                                buildJsonObject {
                                    put("outbounds", JsonArray(listOf(updatedOutbound)))
                                }
                            } else {
                                updatedOutbound
                            }

                            val jsonStringFormatter = Json { prettyPrint = true }
                            val outString = jsonStringFormatter.encodeToString(finalJson)

                            viewModel.saveNodeConfigContent(
                                nodeRef,
                                outString
                            ) { success ->
                                if (success) {
                                    onBack()
                                } else {
                                    notice = saveFailedCheckPermission
                                    noticeId++
                                }
                            }
                        })
                    }
                )
            }
        }
    ) { innerPadding ->
        Box(modifier = if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier) {
            LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .scrollEndHaptic()
                    .overScrollVertical()
                    .nestedScroll(scrollBehavior.nestedScrollConnection),
                contentPadding = innerPadding,
                overscrollEffect = null
            ) {
                item {
                    ServerConfigSection(
                        tag = tag,
                        onTagChange = { tag = it },
                        server = server,
                        onServerChange = { server = it },
                        serverPort = serverPort,
                        onServerPortChange = { serverPort = it },
                        type = type,
                        onTypeChange = { type = it },
                        uuid = uuid,
                        onUuidChange = { uuid = it },
                        flow = flow,
                        onFlowChange = { flow = it },
                        security = security,
                        onSecurityChange = { security = it },
                        alterId = alterId,
                        onAlterIdChange = { alterId = it },
                        method = method,
                        onMethodChange = { method = it },
                        password = password,
                        onPasswordChange = { password = it },
                        plugin = plugin,
                        onPluginChange = { plugin = it },
                        pluginOpts = pluginOpts,
                        onPluginOptsChange = { pluginOpts = it },
                        upMbps = upMbps,
                        onUpMbpsChange = { upMbps = it },
                        downMbps = downMbps,
                        onDownMbpsChange = { downMbps = it },
                        obfsType = obfsType,
                        onObfsTypeChange = { obfsType = it },
                        obfsPassword = obfsPassword,
                        onObfsPasswordChange = { obfsPassword = it },
                        serverPorts = serverPorts,
                        onServerPortsChange = { serverPorts = it },
                        hopInterval = hopInterval,
                        onHopIntervalChange = { hopInterval = it },
                        congestionControl = congestionControl,
                        onCongestionControlChange = { congestionControl = it },
                        udpRelayMode = udpRelayMode,
                        onUdpRelayModeChange = { udpRelayMode = it },
                        udpOverStream = udpOverStream,
                        onUdpOverStreamChange = { udpOverStream = it },
                        zeroRttHandshake = zeroRttHandshake,
                        onZeroRttHandshakeChange = { zeroRttHandshake = it },
                        heartbeat = heartbeat,
                        onHeartbeatChange = { heartbeat = it },
                        focusManager = focusManager,
                        alterIdLabel = alterIdLabel,
                        udpRelayModeTitle = udpRelayModeTitle
                    )
                }

                item {
                    TransportSection(
                        transportType = transportType,
                        path = path,
                        host = host,
                        serviceName = serviceName,
                        onTransportTypeChange = { transportType = it },
                        onPathChange = { path = it },
                        onHostChange = { host = it },
                        onServiceNameChange = { serviceName = it },
                        onImeDone = { focusManager.clearFocus() }
                    )
                }

                item {
                    TlsConfigSection(
                        enabled = tlsEnabled,
                        serverName = serverName,
                        insecure = insecure,
                        disableSni = disableSni,
                        alpn = alpn,
                        fingerprint = fingerprint,
                        realityEnabled = realityEnabled,
                        realityPublicKey = realityPublicKey,
                        realityShortId = realityShortId,
                        echEnabled = echEnabled,
                        echConfig = echConfig,
                        echQueryServerName = echQueryServerName,
                        alpnLabel = alpnLabel,
                        utlsFingerprintLabel = utlsFingerprint,
                        realityPublicKeyLabel = realityPublicKeyLabel,
                        realityShortIdLabel = realityShortIdLabel,
                        echConfigLabel = echConfigListLabel,
                        echDnsServerNameLabel = echDnsServerNameLabel,
                        onEnabledChange = { tlsEnabled = it },
                        onServerNameChange = { serverName = it },
                        onInsecureChange = { insecure = it },
                        onDisableSniChange = { disableSni = it },
                        onAlpnChange = { alpn = it },
                        onFingerprintChange = { fingerprint = it },
                        onRealityEnabledChange = { realityEnabled = it },
                        onRealityPublicKeyChange = { realityPublicKey = it },
                        onRealityShortIdChange = { realityShortId = it },
                        onEchEnabledChange = { echEnabled = it },
                        onEchConfigChange = { echConfig = it },
                        onEchQueryServerNameChange = { echQueryServerName = it },
                        onImeDone = { focusManager.clearFocus() }
                    )
                }

                // 底部占位
                item {
                    Spacer(modifier = Modifier.height(24.dp))
                }
            }
        }
    }
}


