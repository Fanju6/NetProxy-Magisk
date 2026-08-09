package com.fanjv.netproxy.feature.kernel.presentation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Code
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AdaptiveTopAppBar
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.EmptyCatalog
import com.fanjv.netproxy.core.ui.component.TopBarMenuAction
import com.fanjv.netproxy.core.ui.component.TopBarMoreMenu
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.navigation.LocalNavigator
import com.fanjv.netproxy.navigation.Route
import kotlinx.coroutines.launch
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SnackbarDuration
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import top.yukonga.miuix.kmp.utils.scrollEndHaptic

/** sing-box 配置工作台：配置文件是唯一编辑入口。 */
@Composable
internal fun SingBoxKernelSettingsScreen(
    viewModel: SingBoxConfigViewModel = com.fanjv.netproxy.core.di.netProxyViewModel(),
    onBack: () -> Unit,
) {
    val navigator = LocalNavigator.current
    val snackbarHostState = rememberAppSnackbarHostState()
    val coroutineScope = rememberCoroutineScope()
    val state by viewModel.state.collectAsStateWithLifecycle()
    val scrollBehavior = MiuixScrollBehavior()
    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else colorScheme.surface

    LaunchedEffect(Unit) { viewModel.refreshDocuments() }

    val configDocuments = state.documents.filter {
        it.category == SingBoxDocumentCategory.Config
    }
    val commonNames = setOf("03_dns.json", "04_inbounds.json", "06_route.json")
    val commonDocuments = configDocuments.filter { it.filename in commonNames }
    val advancedDocuments = configDocuments.filterNot { it.filename in commonNames }
    val sourceDocuments = state.documents.filter {
        it.category == SingBoxDocumentCategory.Source
    }
    val runtimeDocuments = state.documents.filter {
        it.category == SingBoxDocumentCategory.Runtime
    }
    val checkSuccess = stringResource(R.string.singbox_check_success)
    val checkFailed = stringResource(R.string.singbox_check_failed)

    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbarHostState) },
        topBar = {
            BlurredBar(backdrop) {
                AdaptiveTopAppBar(
                    color = barColor,
                    title = stringResource(R.string.kernel_settings),
                    navigationIcon = { BackIconButton(onClick = onBack) },
                    scrollBehavior = scrollBehavior,
                    actions = {
                        var showMoreMenu by remember { mutableStateOf(false) }
                        TopBarMoreMenu(
                            expanded = showMoreMenu,
                            onExpandedChange = { showMoreMenu = it },
                            actions = listOf(
                                TopBarMenuAction(
                                    text = stringResource(R.string.singbox_check_all),
                                    onClick = {
                                        viewModel.checkConfig { success ->
                                            coroutineScope.launch {
                                                snackbarHostState.showSnackbar(
                                                    message = if (success) checkSuccess else checkFailed,
                                                    withDismissAction = !success,
                                                    duration = if (success) {
                                                        SnackbarDuration.Short
                                                    } else {
                                                        SnackbarDuration.Long
                                                    }
                                                )
                                            }
                                        }
                                    },
                                ),
                                TopBarMenuAction(
                                    text = stringResource(R.string.ebpf_restart_service),
                                    onClick = viewModel::restartService,
                                ),
                            ),
                            contentDescription = stringResource(R.string.more_actions),
                        )
                    },
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
            .only(WindowInsetsSides.Horizontal),
    ) { innerPadding ->
        Box(
            modifier = Modifier
                .fillMaxHeight()
                .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier),
        ) {
            if (state.isLoadingDocuments && state.documents.isEmpty()) {
                Box(
                    modifier = Modifier
                        .fillMaxHeight()
                        .fillMaxWidth()
                        .padding(innerPadding),
                    contentAlignment = Alignment.Center,
                ) {
                    InfiniteProgressIndicator()
                }
            } else if (state.documentsError || state.documents.isEmpty()) {
                Box(
                    modifier = Modifier
                        .fillMaxHeight()
                        .fillMaxWidth()
                        .padding(innerPadding),
                    contentAlignment = Alignment.Center,
                ) {
                    EmptyCatalog(
                        text = stringResource(
                            if (state.documentsError) R.string.singbox_documents_load_failed
                            else R.string.singbox_documents_empty,
                        ),
                        onRefresh = null,
                        modifier = Modifier.padding(horizontal = 24.dp),
                    )
                }
            } else {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxHeight()
                        .scrollEndHaptic()
                        .overScrollVertical()
                        .nestedScroll(scrollBehavior.nestedScrollConnection)
                        .padding(horizontal = 12.dp),
                    contentPadding = innerPadding,
                    overscrollEffect = null,
                ) {
                    if (commonDocuments.isNotEmpty()) {
                        documentSection(
                            keyPrefix = "singbox_common",
                            title = { stringResource(R.string.singbox_common_configs) },
                            documents = commonDocuments,
                            onOpen = { navigator.push(Route.JsonEdit(it.id)) },
                        )
                    }
                    if (advancedDocuments.isNotEmpty()) {
                        documentSection(
                            keyPrefix = "singbox_advanced",
                            title = { stringResource(R.string.singbox_advanced_configs) },
                            documents = advancedDocuments,
                            onOpen = { navigator.push(Route.JsonEdit(it.id)) },
                        )
                    }
                    if (sourceDocuments.isNotEmpty()) {
                        documentSection(
                            keyPrefix = "singbox_source",
                            title = { stringResource(R.string.singbox_rule_files) },
                            documents = sourceDocuments,
                            onOpen = { navigator.push(Route.JsonEdit(it.id)) },
                        )
                    }
                    if (runtimeDocuments.isNotEmpty()) {
                        documentSection(
                            keyPrefix = "singbox_runtime",
                            title = { stringResource(R.string.singbox_runtime_configs) },
                            documents = runtimeDocuments,
                            onOpen = { navigator.push(Route.JsonEdit(it.id)) },
                        )
                    }
                    item { Spacer(Modifier.height(80.dp)) }
                }
            }
        }
    }
}

private fun androidx.compose.foundation.lazy.LazyListScope.documentSection(
    keyPrefix: String,
    title: @Composable () -> String,
    documents: List<SingBoxDocument>,
    onOpen: (SingBoxDocument) -> Unit,
) {
    groupedCardSection(
        keyPrefix = keyPrefix,
        title = title,
        items = documents.map { document ->
            CardItem(document.id) {
                ArrowPreference(
                    title = documentTitle(document),
                    summary = documentSummary(document),
                    startAction = {
                        Icon(
                            imageVector = Icons.Rounded.Code,
                            contentDescription = null,
                            modifier = Modifier.padding(end = 6.dp),
                            tint = colorScheme.onBackground,
                        )
                    },
                    onClick = { onOpen(document) },
                )
            }
        },
    )
}

@Composable
private fun documentTitle(document: SingBoxDocument): String = when (document.filename) {
    "01_log.json" -> stringResource(R.string.singbox_document_log)
    "02_experimental.json" -> stringResource(R.string.singbox_document_experimental)
    "03_dns.json" -> stringResource(R.string.singbox_document_dns)
    "04_inbounds.json" -> stringResource(R.string.singbox_document_inbounds)
    "05_providers.json" -> stringResource(R.string.singbox_document_providers)
    "06_route.json" -> stringResource(R.string.singbox_document_route)
    "07_http_clients.json" -> stringResource(R.string.singbox_document_http_clients)
    "08_services.json" -> stringResource(R.string.singbox_document_services)
    else -> document.filename
}

@Composable
private fun documentSummary(document: SingBoxDocument): String {
    val description = when (document.filename) {
        "01_log.json" -> stringResource(R.string.singbox_document_log_summary)
        "02_experimental.json" -> stringResource(R.string.singbox_document_experimental_summary)
        "03_dns.json" -> stringResource(R.string.singbox_document_dns_summary)
        "04_inbounds.json" -> stringResource(R.string.singbox_document_inbounds_summary)
        "05_providers.json" -> stringResource(R.string.singbox_document_providers_summary)
        "06_route.json" -> stringResource(R.string.singbox_document_route_summary)
        "07_http_clients.json" -> stringResource(R.string.singbox_document_http_clients_summary)
        "08_services.json" -> stringResource(R.string.singbox_document_services_summary)
        else -> when (document.category) {
            SingBoxDocumentCategory.Source -> stringResource(R.string.singbox_document_source_summary)
            SingBoxDocumentCategory.Runtime -> stringResource(R.string.singbox_document_runtime_summary)
            SingBoxDocumentCategory.Config -> stringResource(R.string.singbox_document_custom_summary)
        }
    }
    return if (document.editable) {
        "${document.filename} · $description"
    } else {
        "${document.filename} · ${stringResource(R.string.read_only)} · $description"
    }
}

