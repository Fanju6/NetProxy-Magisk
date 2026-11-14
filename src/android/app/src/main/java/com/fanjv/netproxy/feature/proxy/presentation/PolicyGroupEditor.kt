package com.fanjv.netproxy.feature.proxy.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.resolve
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Icon
import com.fanjv.netproxy.core.ui.component.AppBarButton as IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.ExpandLess
import top.yukonga.miuix.kmp.icon.extended.ExpandMore
import top.yukonga.miuix.kmp.icon.extended.Ok
import com.fanjv.netproxy.core.ui.component.AppDialog as OverlayDialog
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.CheckboxPreference
import com.fanjv.netproxy.core.ui.component.AppDropdown as OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

private enum class PolicyPicker(val title: Int) {
    Members(R.string.policy_group_members),
    Providers(R.string.policy_group_providers),
}

@Composable
internal fun PolicyGroupEditorScreen(
    tag: String?,
    onBack: () -> Unit,
    viewModel: PolicyGroupEditorViewModel = netProxyViewModel(),
) {
    val state by viewModel.editor.collectAsStateWithLifecycle()
    LaunchedEffect(tag) { viewModel.openEditor(tag) }
    LaunchedEffect(state?.saved) { if (state?.saved == true) onBack() }

    val draft = state?.draft
    val document = state?.document
    val editing = tag != null
    val saving = state?.saving == true
    val enabled = !saving && state?.conflict != true
    val error = state?.error?.resolve().orEmpty()
    val change = viewModel::changeGroupDraft
    val scroll = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val focus = LocalFocusManager.current
    var picker by rememberSaveable { mutableStateOf<PolicyPicker?>(null) }
    var advanced by rememberSaveable { mutableStateOf(false) }
    BackHandler(saving) { }

    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    title = stringResource(if (editing) R.string.policy_edit_group else R.string.policy_add_group),
                    color = if (backdrop != null) Color.Transparent else colorScheme.surface,
                    scrollBehavior = scroll,
                    navigationIcon = { BackIconButton { if (!saving) onBack() } },
                    actions = {
                        IconButton(
                            enabled = state?.canSave == true,
                            onClick = {
                                focus.clearFocus()
                                viewModel.saveEditor()
                            },
                        ) {
                            if (saving) InfiniteProgressIndicator(Modifier.size(20.dp))
                            else Icon(MiuixIcons.Ok, stringResource(R.string.save_text), tint = colorScheme.onSurface)
                        }
                    },
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
            .only(WindowInsetsSides.Horizontal + WindowInsetsSides.Bottom),
    ) { padding ->
        Box(Modifier.fillMaxSize().then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
            LazyColumn(
                modifier = Modifier.fillMaxSize().imePadding().scrollEndHaptic().overScrollVertical()
                    .nestedScroll(scroll.nestedScrollConnection).padding(horizontal = 12.dp),
                contentPadding = padding,
                overscrollEffect = null,
            ) {
                if (state?.loading != false) {
                    item("progress") {
                        Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                            InfiniteProgressIndicator()
                        }
                    }
                }
                if (error.isNotBlank()) {
                    item("error") {
                        Text(error, Modifier.padding(horizontal = 14.dp, vertical = 12.dp), color = colorScheme.error)
                    }
                }
                if (draft == null || document == null) return@LazyColumn

                groupedCardSection("basics", { stringResource(R.string.basic_settings) }, buildList {
                    if (editing) {
                        add(CardItem("name") {
                            BasicComponent(title = draft.tag, summary = policyTypeLabel(draft.type))
                        })
                    } else {
                        add(CardItem("name") {
                            PolicyField(R.string.policy_group_name, draft.tag, enabled, first = true) { value ->
                                change { it.copy(tag = value) }
                            }
                            if (draft.tag.trim() in document.memberTags) {
                                Text(
                                    stringResource(R.string.policy_group_duplicate),
                                    Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                                    color = colorScheme.error,
                                    style = MiuixTheme.textStyles.footnote1,
                                )
                            }
                        })
                        add(CardItem("type") {
                            val types = listOf("selector", "urltest")
                            OverlayDropdownPreference(
                                title = stringResource(R.string.policy_group_type),
                                items = types.map(::policyTypeLabel),
                                selectedIndex = types.indexOf(draft.type).coerceAtLeast(0),
                                enabled = enabled,
                                onSelectedIndexChange = { index -> change { it.copy(type = types[index]) } },
                            )
                        })
                    }
                }, titleTopPadding = 12.dp)

                groupedCardSection("sources", { stringResource(R.string.policy_group_sources) }, buildList {
                    if (document.canAddAuto) add(CardItem("add_auto") {
                        SwitchPreference(
                            title = stringResource(R.string.policy_add_auto),
                            summary = stringResource(R.string.policy_add_auto_summary),
                            checked = draft.addAuto,
                            enabled = enabled,
                            onCheckedChange = { value -> change { it.copy(addAuto = value) } },
                        )
                    })
                    add(CardItem("members") {
                        PolicyPickerPreference(PolicyPicker.Members, draft.outbounds.size, enabled) {
                            focus.clearFocus()
                            picker = PolicyPicker.Members
                        }
                    })
                    add(CardItem("all_providers") {
                        SwitchPreference(
                            title = stringResource(R.string.policy_group_all_providers),
                            checked = draft.useAll,
                            enabled = enabled,
                            onCheckedChange = { value -> change { it.copy(useAll = value) } },
                        )
                    })
                    if (!draft.useAll) add(CardItem("providers") {
                        PolicyPickerPreference(PolicyPicker.Providers, draft.providers.size, enabled) {
                            focus.clearFocus()
                            picker = PolicyPicker.Providers
                        }
                    })
                })

                if (draft.type == "selector") {
                    groupedCardSection("behavior", { stringResource(R.string.policy_group_behavior) }, listOf(CardItem("default") {
                        val defaults = (listOf("") + draft.outbounds + draft.defaultMember).distinct()
                        OverlayDropdownPreference(
                            title = stringResource(R.string.policy_group_default),
                            items = defaults.map { it.ifEmpty { stringResource(R.string.policy_group_default_auto) } },
                            selectedIndex = defaults.indexOf(draft.defaultMember).coerceAtLeast(0),
                            enabled = enabled,
                            onSelectedIndexChange = { index -> change { it.copy(defaultMember = defaults[index]) } },
                        )
                    }))
                } else if (draft.type == "urltest") {
                    groupedCardSection("behavior", { stringResource(R.string.policy_group_behavior) }, listOf(
                        CardItem("url") {
                            PolicyField(R.string.policy_group_test_url, draft.testUrl, enabled, first = true) { value ->
                                change { it.copy(testUrl = value.trim()) }
                            }
                        },
                        CardItem("interval") {
                            PolicyField(R.string.policy_group_test_interval, draft.testInterval, enabled) { value ->
                                change { it.copy(testInterval = value.trim()) }
                            }
                        },
                    ))
                }

                if (draft.useAll || draft.providers.isNotEmpty() || draft.include.isNotEmpty() || draft.exclude.isNotEmpty()) {
                    groupedCardSection("advanced", { stringResource(R.string.inbound_advanced) }, buildList {
                        add(CardItem("filters") {
                            BasicComponent(
                                title = stringResource(R.string.policy_group_filters),
                                endActions = {
                                    Icon(
                                        if (advanced) MiuixIcons.ExpandLess else MiuixIcons.ExpandMore,
                                        contentDescription = null,
                                        tint = colorScheme.onSurfaceVariantActions,
                                    )
                                },
                                onClick = { advanced = !advanced },
                            )
                        })
                        if (advanced) {
                            add(CardItem("include") {
                                PolicyField(R.string.policy_group_include, draft.include, enabled) { value ->
                                    change { it.copy(include = value) }
                                }
                            })
                            add(CardItem("exclude") {
                                PolicyField(R.string.policy_group_exclude, draft.exclude, enabled) { value ->
                                    change { it.copy(exclude = value) }
                                }
                            })
                        }
                    })
                }
                item("bottom") { Spacer(Modifier.height(16.dp)) }
            }
        }
    }

    if (draft != null && document != null) picker?.let { kind ->
        val members = kind == PolicyPicker.Members
        PolicyChoicesDialog(
            kind = kind,
            selected = if (members) draft.outbounds else draft.providers,
            choices = if (members) {
                document.memberTags.filter { it != (tag ?: draft.tag.trim()) }
            } else document.providerChoices,
            onDismiss = { picker = null },
            onApply = { next ->
                if (members) change { current ->
                    current.copy(
                        outbounds = next,
                        defaultMember = current.defaultMember.takeUnless { it in current.outbounds && it !in next }.orEmpty(),
                    )
                } else change { it.copy(providers = next) }
                picker = null
            },
        )
    }
}

@Composable
private fun PolicyField(
    label: Int, value: String, enabled: Boolean, first: Boolean = false, onChange: (String) -> Unit,
) {
    TextField(
        value = value,
        onValueChange = onChange,
        label = stringResource(label),
        enabled = enabled,
        singleLine = true,
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, top = if (first) 12.dp else 0.dp, end = 12.dp, bottom = 12.dp),
    )
}

@Composable
private fun PolicyPickerPreference(kind: PolicyPicker, count: Int, enabled: Boolean, onClick: () -> Unit) {
    ArrowPreference(
        title = stringResource(kind.title),
        summary = if (count == 0) stringResource(R.string.policy_group_no_sources)
            else stringResource(R.string.policy_group_selected_count, count),
        enabled = enabled,
        onClick = onClick,
    )
}

@Composable
private fun PolicyChoicesDialog(
    kind: PolicyPicker,
    selected: List<String>,
    choices: List<String>,
    onDismiss: () -> Unit,
    onApply: (List<String>) -> Unit,
) {
    var checked by rememberSaveable(kind, selected) { mutableStateOf(ArrayList(selected)) }
    val entries = (choices + selected).distinct()
    OverlayDialog(show = true, title = stringResource(kind.title), onDismissRequest = onDismiss) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (entries.isEmpty()) {
                Text(stringResource(R.string.policy_group_no_choices), color = colorScheme.onSurfaceVariantSummary)
            } else {
                LazyColumn(Modifier.heightIn(max = 360.dp).weight(1f, fill = false), overscrollEffect = null) {
                    items(entries, key = { it }) { choice ->
                        CheckboxPreference(
                            title = choice,
                            checked = choice in checked,
                            insideMargin = PaddingValues(vertical = 12.dp),
                            onCheckedChange = { value ->
                                checked = ArrayList(if (value) checked + choice else checked - choice)
                            },
                        )
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                TextButton(stringResource(R.string.common_cancel), modifier = Modifier.weight(1f), onClick = onDismiss)
                TextButton(
                    stringResource(R.string.common_done), modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.textButtonColorsPrimary(), onClick = { onApply(checked) },
                )
            }
        }
    }
}

private fun policyTypeLabel(type: String): String = when (type) {
    "selector" -> "Selector"
    "urltest" -> "URLTest"
    "loadbalance" -> "LoadBalance"
    else -> type
}
