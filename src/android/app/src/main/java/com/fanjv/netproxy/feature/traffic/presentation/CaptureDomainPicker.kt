package com.fanjv.netproxy.feature.traffic.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
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
import com.fanjv.netproxy.feature.traffic.data.CaptureDomain
import kotlinx.coroutines.CancellationException
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.glass.GlassTextField
import top.yukonga.miuix.kmp.preference.ArrowPreference

@Composable
internal fun CaptureDomainPicker(
    viewModel: TrafficViewModel,
    batch: String?,
    selected: String?,
    allowAll: Boolean,
    onDismiss: () -> Unit,
    choose: (String?) -> Unit,
) {
    var query by rememberSaveable { mutableStateOf("") }
    var domains by remember(batch) { mutableStateOf<List<CaptureDomain>?>(null) }
    var error by remember(batch) { mutableStateOf<Throwable?>(null) }
    LaunchedEffect(viewModel, batch) {
        try {
            domains = viewModel.capturedDomains(batch)
        } catch (failure: CancellationException) {
            throw failure
        } catch (failure: Exception) {
            error = failure
        }
    }
    val options = remember(domains, query) { domains.orEmpty().filter { it.host.contains(query.trim(), ignoreCase = true) } }
    fun select(host: String?) {
        choose(host)
        onDismiss()
    }
    AppDialog(show = true, title = stringResource(if (allowAll) R.string.traffic_domain_filter else R.string.traffic_rule_pick_domain),
        onDismissRequest = onDismiss) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            GlassTextField(query, { query = it }, modifier = Modifier.fillMaxWidth(),
                label = stringResource(R.string.traffic_domain_search), singleLine = true)
            if (allowAll) ArrowPreference(title = stringResource(R.string.traffic_filter_all),
                enabled = selected != null, onClick = { select(null) }, insideMargin = TrafficRowInsets)
            when {
                error != null -> TrafficErrorText(error!!)
                domains == null -> InfiniteProgressIndicator()
                options.isEmpty() -> Text(stringResource(R.string.traffic_no_domains))
                else -> LazyColumn(Modifier.heightIn(max = 280.dp)) {
                    items(options, key = { it.host }) { domain ->
                        ArrowPreference(title = domain.host, summary = stringResource(R.string.traffic_record_count, domain.count),
                            enabled = selected != domain.host, onClick = { select(domain.host) }, insideMargin = TrafficRowInsets)
                    }
                }
            }
        }
    }
}
