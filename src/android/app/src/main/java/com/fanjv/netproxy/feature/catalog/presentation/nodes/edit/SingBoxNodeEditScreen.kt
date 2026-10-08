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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel as nodeEditViewModel
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
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.serialization.json.Json
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

    val editor: NodeEditViewModel = nodeEditViewModel(key = "node-edit/$nodeRef")
    val nodeDraft by editor.draft.collectAsStateWithLifecycle()
    val draft = nodeDraft?.fields ?: NodeEditFields()

    LaunchedEffect(nodeRef) {
        if (editor.draft.value != null) return@LaunchedEffect
        try {
            val content = viewModel.loadNodeConfigContent(nodeRef)
            currentCoroutineContext().ensureActive()
            editor.initialize(content)
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            notice = saveFailedCheckPermission
            noticeId++
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
                            if (draft.tag.isBlank()) {
                                notice = nodeTagEmpty
                                noticeId++
                                return@ActionButtons
                            }
                            if (draft.server.isBlank()) {
                                notice = serverAddressEmpty
                                noticeId++
                                return@ActionButtons
                            }
                            if (draft.serverPort.isBlank()) {
                                notice = serverPortEmpty
                                noticeId++
                                return@ActionButtons
                            }

                            val snapshot = editor.draft.value ?: return@ActionButtons
                            val jsonStringFormatter = Json { prettyPrint = true }
                            val outString = jsonStringFormatter.encodeToString(snapshot.toJson())

                            viewModel.saveNodeConfigContent(
                                nodeRef,
                                outString
                            ) { success ->
                                if (success && editor.draft.value == snapshot) {
                                    onBack()
                                } else if (!success) {
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
                        tag = draft.tag,
                        onTagChange = { value -> editor.update { it.copy(tag = value) } },
                        server = draft.server,
                        onServerChange = { value -> editor.update { it.copy(server = value) } },
                        serverPort = draft.serverPort,
                        onServerPortChange = { value -> editor.update { it.copy(serverPort = value) } },
                        type = draft.type,
                        onTypeChange = { value -> editor.update { it.copy(type = value) } },
                        uuid = draft.uuid,
                        onUuidChange = { value -> editor.update { it.copy(uuid = value) } },
                        flow = draft.flow,
                        onFlowChange = { value -> editor.update { it.copy(flow = value) } },
                        security = draft.security,
                        onSecurityChange = { value -> editor.update { it.copy(security = value) } },
                        alterId = draft.alterId,
                        onAlterIdChange = { value -> editor.update { it.copy(alterId = value) } },
                        method = draft.method,
                        onMethodChange = { value -> editor.update { it.copy(method = value) } },
                        password = draft.password,
                        onPasswordChange = { value -> editor.update { it.copy(password = value) } },
                        plugin = draft.plugin,
                        onPluginChange = { value -> editor.update { it.copy(plugin = value) } },
                        pluginOpts = draft.pluginOpts,
                        onPluginOptsChange = { value -> editor.update { it.copy(pluginOpts = value) } },
                        upMbps = draft.upMbps,
                        onUpMbpsChange = { value -> editor.update { it.copy(upMbps = value) } },
                        downMbps = draft.downMbps,
                        onDownMbpsChange = { value -> editor.update { it.copy(downMbps = value) } },
                        obfsType = draft.obfsType,
                        onObfsTypeChange = { value -> editor.update { it.copy(obfsType = value) } },
                        obfsPassword = draft.obfsPassword,
                        onObfsPasswordChange = { value -> editor.update { it.copy(obfsPassword = value) } },
                        serverPorts = draft.serverPorts,
                        onServerPortsChange = { value -> editor.update { it.copy(serverPorts = value) } },
                        hopInterval = draft.hopInterval,
                        onHopIntervalChange = { value -> editor.update { it.copy(hopInterval = value) } },
                        congestionControl = draft.congestionControl,
                        onCongestionControlChange = { value -> editor.update { it.copy(congestionControl = value) } },
                        udpRelayMode = draft.udpRelayMode,
                        onUdpRelayModeChange = { value -> editor.update { it.copy(udpRelayMode = value) } },
                        udpOverStream = draft.udpOverStream,
                        onUdpOverStreamChange = { value -> editor.update { it.copy(udpOverStream = value) } },
                        zeroRttHandshake = draft.zeroRttHandshake,
                        onZeroRttHandshakeChange = { value -> editor.update { it.copy(zeroRttHandshake = value) } },
                        heartbeat = draft.heartbeat,
                        onHeartbeatChange = { value -> editor.update { it.copy(heartbeat = value) } },
                        focusManager = focusManager,
                        alterIdLabel = alterIdLabel,
                        udpRelayModeTitle = udpRelayModeTitle
                    )
                }

                item {
                    TransportSection(
                        transportType = draft.transportType,
                        path = draft.path,
                        host = draft.host,
                        serviceName = draft.serviceName,
                        onTransportTypeChange = { value -> editor.update { it.copy(transportType = value) } },
                        onPathChange = { value -> editor.update { it.copy(path = value) } },
                        onHostChange = { value -> editor.update { it.copy(host = value) } },
                        onServiceNameChange = { value -> editor.update { it.copy(serviceName = value) } },
                        onImeDone = { focusManager.clearFocus() }
                    )
                }

                item {
                    TlsConfigSection(
                        enabled = draft.tlsEnabled,
                        serverName = draft.serverName,
                        insecure = draft.insecure,
                        disableSni = draft.disableSni,
                        alpn = draft.alpn,
                        fingerprint = draft.fingerprint,
                        realityEnabled = draft.realityEnabled,
                        realityPublicKey = draft.realityPublicKey,
                        realityShortId = draft.realityShortId,
                        echEnabled = draft.echEnabled,
                        echConfig = draft.echConfig,
                        echQueryServerName = draft.echQueryServerName,
                        alpnLabel = alpnLabel,
                        utlsFingerprintLabel = utlsFingerprint,
                        realityPublicKeyLabel = realityPublicKeyLabel,
                        realityShortIdLabel = realityShortIdLabel,
                        echConfigLabel = echConfigListLabel,
                        echDnsServerNameLabel = echDnsServerNameLabel,
                        onEnabledChange = { value -> editor.update { it.copy(tlsEnabled = value) } },
                        onServerNameChange = { value -> editor.update { it.copy(serverName = value) } },
                        onInsecureChange = { value -> editor.update { it.copy(insecure = value) } },
                        onDisableSniChange = { value -> editor.update { it.copy(disableSni = value) } },
                        onAlpnChange = { value -> editor.update { it.copy(alpn = value) } },
                        onFingerprintChange = { value -> editor.update { it.copy(fingerprint = value) } },
                        onRealityEnabledChange = { value -> editor.update { it.copy(realityEnabled = value) } },
                        onRealityPublicKeyChange = { value -> editor.update { it.copy(realityPublicKey = value) } },
                        onRealityShortIdChange = { value -> editor.update { it.copy(realityShortId = value) } },
                        onEchEnabledChange = { value -> editor.update { it.copy(echEnabled = value) } },
                        onEchConfigChange = { value -> editor.update { it.copy(echConfig = value) } },
                        onEchQueryServerNameChange = { value -> editor.update { it.copy(echQueryServerName = value) } },
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
