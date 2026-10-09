package com.fanjv.netproxy.feature.settings.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.*
import kotlinx.coroutines.CancellationException
import top.yukonga.miuix.kmp.basic.*
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.CheckboxPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

private data class WiFiEditor(val original: String? = null)

@Composable
internal fun NetworkMatchingScreen(
    onBack: () -> Unit,
    viewModel: SettingsViewModel = netProxyViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val enabled = state.hasLoaded && !state.requiresReload
    val commitOnLeave = rememberCommitOnLeave(viewModel::requestWifiFlush)
    val leave: () -> Unit = { commitOnLeave(onBack) }
    val wifi = state.wifi
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    var editor by remember { mutableStateOf<WiFiEditor?>(null) }
    var showEditor by remember { mutableStateOf(false) }
    fun openEditor(original: String? = null) { editor = WiFiEditor(original); showEditor = true }
    BackHandler(!showEditor && (state.hasPendingWifi || state.isSaving || state.requiresReload)) { leave() }
    LifecycleResumeEffect(Unit) {
        viewModel.refresh()
        onPauseOrDispose {}
    }
    Scaffold(topBar = {
        BlurredBar(backdrop) {
            AdaptiveTopAppBar(
                title = stringResource(R.string.network_matching),
                color = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface,
                scrollBehavior = scrollBehavior,
                navigationIcon = { BackIconButton(onClick = leave) },
                actions = {
                    if (wifi.enabled && state.hasLoaded) IconButton(enabled = enabled, onClick = { openEditor() }) {
                        Icon(MiuixIcons.Add, stringResource(R.string.wifi_add))
                    }
                },
            )
        }
    }) { padding ->
        BoxWithConstraints(Modifier.fillMaxSize().then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
            if (!state.hasLoaded) {
                ContentStatus(padding, stringResource(R.string.network_read_failed), loading = state.error.isBlank())
            } else {
                val empty = wifi.enabled && wifi.ssids.isEmpty()
                val contentHeight = (maxHeight - padding.calculateTopPadding() - padding.calculateBottomPadding()).coerceAtLeast(0.dp)
                LazyColumn(
                    modifier = Modifier.fillMaxSize().scrollEndHaptic().overScrollVertical()
                        .nestedScroll(scrollBehavior.nestedScrollConnection).padding(horizontal = 12.dp),
                    contentPadding = padding,
                    overscrollEffect = null,
                ) {
                    item("policy") {
                        // 保留提示文字的自然高度，避免横屏或大字体把剩余内容区压成零。
                        Column(Modifier.fillMaxWidth().then(if (empty)
                            Modifier.heightIn(min = contentHeight).height(IntrinsicSize.Min) else Modifier)) {
                            if (state.error.isNotBlank()) Text(state.error, Modifier.padding(14.dp), color = MiuixTheme.colorScheme.error)
                            if (state.requiresReload) TextButton(
                                stringResource(R.string.routing_reload_draft), onClick = viewModel::discardWifiAndReload,
                            )
                            SmallTitle(stringResource(R.string.wifi_policy_title),
                                insideMargin = PaddingValues(start = 14.dp, top = 20.dp, end = 14.dp, bottom = 8.dp))
                            Card {
                                val modes = listOf("off", "blacklist", "whitelist")
                                OverlayDropdownPreference(
                                    title = stringResource(R.string.wifi_auto_switch),
                                    items = listOf(stringResource(R.string.wifi_mode_off), stringResource(R.string.wifi_ssid_mode_blacklist), stringResource(R.string.wifi_ssid_mode_whitelist)),
                                    selectedIndex = modes.indexOf(wifi.selection),
                                    enabled = enabled,
                                    onSelectedIndexChange = { viewModel.setWifiSsidMode(modes[it]) },
                                )
                                SwitchPreference(
                                    title = stringResource(R.string.proxy_on_non_wifi), checked = wifi.proxyOnNonWifi,
                                    enabled = enabled && wifi.enabled, onCheckedChange = viewModel::setProxyOnNonWifi,
                                )
                            }
                            if (empty) ContentStatus(PaddingValues(bottom = 12.dp), stringResource(R.string.wifi_empty),
                                modifier = Modifier.weight(1f))
                        }
                    }
                    if (wifi.enabled && !empty) groupedCardSection("ssids", {
                        stringResource(if (wifi.mode == "whitelist") R.string.wifi_whitelist_title else R.string.wifi_blacklist_title)
                    }, wifi.ssids.map { name -> CardItem(name) {
                        BasicComponent(enabled = enabled, onClick = { openEditor(name) },
                            holdDownState = showEditor && editor?.original == name) {
                            Text(name, style = MiuixTheme.textStyles.headline1, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        }
                    } })
                    if (!empty) item("bottom") { Spacer(Modifier.height(12.dp)) }
                }
            }
        }
    }
    editor?.let { editing ->
        WiFiNameDialog(
            show = showEditor, original = editing.original, current = wifi.ssids, enabled = enabled,
            loadCandidates = viewModel::savedWifiNetworks,
            onDismiss = { showEditor = false }, onDismissFinished = { editor = null },
            onSave = { viewModel.setWifiSsids(it); showEditor = false },
        )
    }
}

@Composable
private fun WiFiNameDialog(
    show: Boolean, original: String?, current: List<String>, enabled: Boolean,
    loadCandidates: suspend () -> List<String>, onDismiss: () -> Unit,
    onDismissFinished: () -> Unit, onSave: (List<String>) -> Unit,
) {
    var input by remember { mutableStateOf(original.orEmpty()) }
    var selected by remember { mutableStateOf(emptySet<String>()) }
    var candidates by remember { mutableStateOf<List<String>?>(null) }
    var failed by remember { mutableStateOf(false) }
    var invalid by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) {
        if (original == null) try {
            candidates = loadCandidates().filterNot { it in current }
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            failed = true
        }
    }
    OverlayDialog(
        show = show, title = stringResource(if (original == null) R.string.wifi_add else R.string.wifi_edit),
        insideMargin = DpSize(0.dp, 24.dp), onDismissRequest = onDismiss, onDismissFinished = onDismissFinished,
    ) {
        BoxWithConstraints(Modifier.fillMaxWidth().heightIn(max = 520.dp)) {
            Column(verticalArrangement = Arrangement.spacedBy(if (maxHeight < 360.dp) 8.dp else 12.dp)) {
                TextField(value = input, onValueChange = { input = it; invalid = false },
                    label = stringResource(R.string.wifi_name), maxLines = 1,
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp))
                if (invalid) Text(stringResource(R.string.wifi_invalid_name), Modifier.padding(horizontal = 24.dp),
                    style = MiuixTheme.textStyles.footnote1, color = MiuixTheme.colorScheme.error)
                if (original == null) {
                    when {
                        failed -> Text(stringResource(R.string.wifi_saved_failed), Modifier.padding(horizontal = 24.dp),
                            style = MiuixTheme.textStyles.footnote1)
                        candidates == null -> Box(Modifier.fillMaxWidth().height(48.dp), contentAlignment = Alignment.Center) {
                            InfiniteProgressIndicator()
                        }
                        candidates!!.isEmpty() -> Text(stringResource(R.string.wifi_saved_empty), Modifier.padding(horizontal = 24.dp),
                            style = MiuixTheme.textStyles.footnote1)
                        else -> LazyColumn(Modifier.weight(1f, fill = false), overscrollEffect = null) {
                            items(candidates!!, key = { it }) { name ->
                                CheckboxPreference(title = name, checked = name in selected,
                                    insideMargin = PaddingValues(horizontal = 24.dp, vertical = 12.dp),
                                    onCheckedChange = { selected = if (it) selected + name else selected - name })
                            }
                        }
                    }
                } else {
                    TextButton(stringResource(R.string.common_delete),
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp), enabled = enabled,
                        onClick = { onSave(current - original) })
                }
                Row(Modifier.fillMaxWidth().padding(horizontal = 24.dp), horizontalArrangement = Arrangement.spacedBy(20.dp)) {
                    TextButton(stringResource(android.R.string.cancel), modifier = Modifier.weight(1f), onClick = onDismiss)
                    TextButton(stringResource(if (original == null) R.string.common_add else R.string.save_text),
                        modifier = Modifier.weight(1f), enabled = enabled && (input.isNotEmpty() || selected.isNotEmpty()),
                        colors = ButtonDefaults.textButtonColorsPrimary(), onClick = {
                            if (input.isNotEmpty() && (input.toByteArray(Charsets.UTF_8).size > 32 || input.any { it.isISOControl() })) {
                                invalid = true
                            } else {
                                onSave(if (original == null) (current + selected + listOfNotNull(input.takeIf(String::isNotEmpty))).distinct()
                                    else current.map { if (it == original) input else it }.distinct())
                            }
                        })
                }
            }
        }
    }
}
