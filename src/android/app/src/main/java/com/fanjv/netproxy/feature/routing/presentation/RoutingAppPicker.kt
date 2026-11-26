package com.fanjv.netproxy.feature.routing.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.input.TextFieldLineLimits
import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.flowWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.dialogTextFieldColors
import com.fanjv.netproxy.feature.apps.data.AppIconCache
import com.fanjv.netproxy.feature.apps.presentation.AppIcon
import com.fanjv.netproxy.feature.apps.presentation.AppInfoModel
import com.fanjv.netproxy.feature.routing.model.RuleField
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.collectLatest
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.CheckboxLocation
import top.yukonga.miuix.kmp.preference.CheckboxPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme

@Composable
internal fun RoutingAppPicker(
    initialText: String,
    viewModel: RoutingRulesViewModel,
    onCancel: () -> Unit,
    onDone: (String) -> Unit,
) {
    val search = rememberTextFieldState()
    val query = search.text.toString().trim()
    val listState = rememberLazyListState()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val initialSelection = remember(initialText) { RuleField.PackageName.values(initialText) }
    var selection by rememberSaveable { mutableStateOf(ArrayList(initialSelection)) }
    val selected = remember(selection) { selection.toSet() }
    var apps by remember { mutableStateOf<Result<List<AppInfoModel>>?>(null) }
    var loadAttempt by remember { mutableIntStateOf(0) }
    LaunchedEffect(viewModel, loadAttempt) {
        apps = null
        apps = try {
            Result.success(viewModel.installedApps())
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            Result.failure(error)
        }
    }
    val installed = apps?.getOrNull().orEmpty()
    val icons = remember(installed) { installed.associate { it.packageName to it.iconKey } }
    val matches = remember(installed, query) {
        if (query.isBlank()) installed else installed.filter {
            it.label.contains(query, ignoreCase = true) || it.packageName.contains(query, ignoreCase = true)
        }
    }
    LaunchedEffect(query) { listState.scrollToItem(0) }
    LaunchedEffect(listState, lifecycle, icons) {
        snapshotFlow {
            listState.layoutInfo.visibleItemsInfo.mapNotNull { (it.key as? String)?.let(icons::get) } to AppIconCache.generation
        }.flowWithLifecycle(lifecycle, Lifecycle.State.RESUMED)
            .collectLatest { (keys, _) -> viewModel.loadAppIcons(keys) }
    }

    Column(Modifier.heightIn(max = 500.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        GlassTextField(
            colors = dialogTextFieldColors(),
            state = search,
            label = stringResource(R.string.search),
            modifier = Modifier.fillMaxWidth(),
            lineLimits = TextFieldLineLimits.SingleLine,
            keyboardOptions = KeyboardOptions(autoCorrectEnabled = false),
        )
        LazyColumn(
            state = listState,
            modifier = Modifier.weight(1f, fill = false).fillMaxWidth(),
            overscrollEffect = null,
        ) {
            when {
                apps == null -> item("loading") {
                    Box(Modifier.fillMaxWidth().padding(vertical = 16.dp), contentAlignment = Alignment.Center) {
                        InfiniteProgressIndicator()
                    }
                }
                apps?.isFailure == true -> item("error") {
                    ArrowPreference(
                        title = stringResource(R.string.routing_retry_apps),
                        summary = stringResource(R.string.routing_apps_unavailable),
                        onClick = { loadAttempt++ },
                    )
                }
                matches.isEmpty() -> item("empty") {
                    val label = if (installed.isEmpty()) R.string.routing_apps_empty else R.string.search_no_results
                    Text(stringResource(label), Modifier.padding(vertical = 12.dp),
                        style = MiuixTheme.textStyles.body2, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                }
                else -> items(matches, key = AppInfoModel::packageName, contentType = { "app" }) { app ->
                    CheckboxPreference(
                        title = app.label,
                        summary = app.packageName.takeUnless { it == app.label },
                        startAction = { AppIcon(app, Modifier.padding(end = 12.dp)) },
                        checked = app.packageName in selected,
                        checkboxLocation = CheckboxLocation.End,
                        insideMargin = PaddingValues(vertical = 10.dp),
                        onCheckedChange = { checked ->
                            selection = ArrayList(if (checked) (selection + app.packageName).distinct() else selection - app.packageName)
                        },
                    )
                }
            }
        }
        Text(stringResource(R.string.routing_apps_scope),
            style = MiuixTheme.textStyles.footnote2, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            AppDialogButton(stringResource(R.string.common_cancel), onCancel, Modifier.weight(1f))
            AppDialogButton(
                stringResource(R.string.common_done), { onDone(selection.joinToString("\n")) },
                Modifier.weight(1f), enabled = selection != initialSelection, role = DialogActionRole.Primary,
            )
        }
    }
}
