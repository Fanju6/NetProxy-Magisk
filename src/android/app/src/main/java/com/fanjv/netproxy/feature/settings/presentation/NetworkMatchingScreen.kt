package com.fanjv.netproxy.feature.settings.presentation

import androidx.compose.foundation.layout.*
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.*
import top.yukonga.miuix.kmp.basic.*
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

@Composable
internal fun NetworkMatchingScreen(
    onBack: () -> Unit,
    viewModel: SettingsViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val enabled = state.hasLoaded && !state.requiresReload
    val commitOnLeave = rememberCommitOnLeave(viewModel::requestWifiFlush)
    val leave: () -> Unit = { commitOnLeave(onBack) }
    val wifi = state.wifi
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    var editing by rememberSaveable { mutableStateOf(false) }
    var ssids by rememberSaveable { mutableStateOf("") }
    BackHandler(enabled = !editing && (state.hasPendingWifi || state.isSaving || state.requiresReload)) { leave() }

    LifecycleResumeEffect(Unit) {
        viewModel.refresh()
        onPauseOrDispose {}
    }

    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    title = stringResource(R.string.network_matching),
                    color = if (backdrop != null) Color.Transparent else colorScheme.surface,
                    scrollBehavior = scrollBehavior,
                    navigationIcon = { BackIconButton(onClick = leave) }
                )
            }
        },
    ) { padding ->
        Box(Modifier.then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
            if (!state.hasLoaded) {
                ContentStatus(padding, stringResource(R.string.network_read_failed), loading = state.error.isBlank())
            } else {
                LazyColumn(
                    modifier = Modifier.fillMaxHeight().scrollEndHaptic().overScrollVertical()
                        .nestedScroll(scrollBehavior.nestedScrollConnection).padding(horizontal = 12.dp),
                    contentPadding = padding,
                    overscrollEffect = null
                ) {
                    if (state.error.isNotBlank()) item("error") {
                        Text(state.error, Modifier.padding(14.dp), color = colorScheme.error)
                    }
                    if (state.requiresReload) item("reload") {
                        TextButton(stringResource(R.string.routing_reload_draft), onClick = viewModel::discardWifiAndReload)
                    }
                    if (state.hasLoaded) groupedCardSection("wifi", { stringResource(R.string.wifi_auto_switch_title) }, listOf(
                        CardItem("enabled") {
                            SwitchPreference(title = stringResource(R.string.wifi_auto_switch), checked = wifi.enabled,
                                enabled = enabled, onCheckedChange = viewModel::setWifiAutoSwitch)
                        },
                        CardItem("mode") {
                            OverlayDropdownPreference(
                                title = stringResource(R.string.wifi_ssid_mode),
                                items = listOf(stringResource(R.string.wifi_ssid_mode_blacklist), stringResource(R.string.wifi_ssid_mode_whitelist)),
                                selectedIndex = if (wifi.mode == "whitelist") 1 else 0,
                                enabled = enabled,
                                onSelectedIndexChange = { viewModel.setWifiSsidMode(if (it == 1) "whitelist" else "blacklist") }
                            )
                        },
                        CardItem("ssids") {
                            ArrowPreference(
                                title = stringResource(R.string.wifi_ssid_list),
                                summary = wifi.ssids.ifBlank { stringResource(R.string.not_set) },
                                enabled = enabled,
                                onClick = { ssids = wifi.ssids; editing = true },
                                holdDownState = editing
                            )
                        },
                        CardItem("cellular") {
                            SwitchPreference(title = stringResource(R.string.proxy_on_cellular), checked = wifi.proxyOnCellular,
                                enabled = enabled, onCheckedChange = viewModel::setProxyOnCellular)
                        }
                    ))
                    if (state.error.isNotBlank() && !state.requiresReload) item("retry") {
                        TextButton(stringResource(R.string.inbound_reload), enabled = !state.isLoading && !state.isSaving,
                            onClick = viewModel::refresh)
                    }
                    item { Spacer(Modifier.height(12.dp)) }
                }
            }
        }
    }
    OverlayDialog(
        show = editing,
        title = stringResource(R.string.modify_label, stringResource(R.string.wifi_ssid_list)),
        onDismissRequest = { editing = false }
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            TextField(value = ssids, onValueChange = { ssids = it },
                label = stringResource(R.string.settings_hint_ssids), modifier = Modifier.fillMaxWidth())
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(20.dp)) {
                TextButton(stringResource(android.R.string.cancel), modifier = Modifier.weight(1f),
                    onClick = { editing = false })
                TextButton(stringResource(R.string.save_text), modifier = Modifier.weight(1f), enabled = enabled,
                    colors = ButtonDefaults.textButtonColorsPrimary(), onClick = {
                        viewModel.setWifiSsidList(ssids)
                        editing = false
                    })
            }
        }
    }
}
