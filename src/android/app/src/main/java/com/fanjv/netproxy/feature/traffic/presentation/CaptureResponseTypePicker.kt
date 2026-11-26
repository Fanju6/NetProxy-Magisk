package com.fanjv.netproxy.feature.traffic.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialog
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.feature.traffic.data.CaptureResponseType
import kotlinx.coroutines.CancellationException
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.preference.SwitchPreference

@Composable
internal fun CaptureResponseTypePicker(
    viewModel: TrafficViewModel,
    batch: String?,
    host: String?,
    selected: Set<String>,
    onDismiss: () -> Unit,
    choose: (Set<String>) -> Unit,
) {
    var draft by rememberSaveable(selected) { mutableStateOf(ArrayList(selected)) }
    var types by remember(batch, host) { mutableStateOf<List<CaptureResponseType>?>(null) }
    var error by remember(batch, host) { mutableStateOf<Throwable?>(null) }
    LaunchedEffect(viewModel, batch, host) {
        try {
            types = viewModel.capturedResponseTypes(batch, host)
        } catch (failure: CancellationException) {
            throw failure
        } catch (failure: Exception) {
            error = failure
        }
    }
    AppDialog(show = true, title = stringResource(R.string.traffic_response_type_filter), onDismissRequest = onDismiss) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            when {
                error != null -> TrafficErrorText(error!!)
                types == null -> InfiniteProgressIndicator()
                else -> LazyColumn(Modifier.heightIn(max = 330.dp)) {
                    items(types.orEmpty(), key = { it.type }) { option ->
                        SwitchPreference(title = responseTypeLabel(option.type),
                            summary = stringResource(R.string.traffic_record_count, option.count),
                            checked = option.type in draft, insideMargin = TrafficRowInsets,
                            onCheckedChange = { checked ->
                                draft = ArrayList(if (checked) draft + option.type else draft - option.type)
                            })
                    }
                }
            }
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                AppDialogButton(stringResource(R.string.traffic_filter_all), {
                    choose(emptySet())
                    onDismiss()
                }, Modifier.weight(1f), enabled = selected.isNotEmpty())
                AppDialogButton(stringResource(R.string.common_confirm), {
                    choose(draft.toSet())
                    onDismiss()
                }, Modifier.weight(1f), enabled = types != null, role = DialogActionRole.Primary)
            }
        }
    }
}

@Composable
private fun responseTypeLabel(type: String): String = when (type) {
    "json" -> "JSON"
    "html" -> "HTML"
    "xml" -> "XML"
    "text" -> stringResource(R.string.traffic_response_type_text)
    "image" -> stringResource(R.string.traffic_response_type_image)
    "video" -> stringResource(R.string.traffic_response_type_video)
    "audio" -> stringResource(R.string.traffic_response_type_audio)
    "binary" -> stringResource(R.string.traffic_response_type_binary)
    "none" -> stringResource(R.string.traffic_response_type_none)
    else -> type
}
