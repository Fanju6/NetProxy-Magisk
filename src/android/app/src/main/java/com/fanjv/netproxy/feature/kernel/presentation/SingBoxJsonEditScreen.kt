package com.fanjv.netproxy.feature.kernel.presentation

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.LocalOverscrollFactory
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.captionBar
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.union
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.selected
import top.yukonga.miuix.kmp.glass.GlassFloatingToolbar
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppDialogButton
import com.fanjv.netproxy.core.ui.component.DialogActionRole
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.JsonSyntaxHighlighter
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.isInDarkTheme
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.launch
import kotlinx.serialization.json.jsonObject
import top.yukonga.miuix.kmp.basic.CircularProgressIndicator
import top.yukonga.miuix.kmp.basic.Icon
import com.fanjv.netproxy.core.ui.component.AppBarButton as IconButton
import top.yukonga.miuix.kmp.basic.Scaffold
import com.fanjv.netproxy.core.ui.component.AppSmallTopAppBar as SmallTopAppBar
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.blur.layerBackdrop
import com.fanjv.netproxy.core.ui.component.chromeIcon
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Ok
import com.fanjv.netproxy.core.ui.component.AppDialog as OverlayDialog
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.scripta.editor.CodeEditor
import top.yukonga.scripta.editor.EditorColors
import top.yukonga.scripta.editor.EditorLanguage
import top.yukonga.scripta.editor.EditorSymbol
import top.yukonga.scripta.editor.LocalEditorBottomContentInset
import top.yukonga.scripta.editor.rememberSaveableCodeEditorController
import top.yukonga.scripta.editor.text.TextPosition

/** 使用虚拟化代码编辑器修改 sing-box JSON 配置。 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun SingBoxJsonEditScreen(
    viewModel: SingBoxConfigViewModel = com.fanjv.netproxy.core.di.netProxyViewModel(),
    documentId: String,
    onBack: () -> Unit,
) {
    val configState by viewModel.state.collectAsStateWithLifecycle()
    val controller = rememberSaveableCodeEditorController()
    val highlighter = remember { JsonSyntaxHighlighter() }
    val context = LocalContext.current
    val snackbarHostState = rememberAppSnackbarHostState()
    val resources = LocalResources.current
    val languageTags = LocalConfiguration.current.locales.toLanguageTags()
    val schemaContext = remember(context, languageTags) {
        context.createConfigurationContext(resources.configuration)
    }
    val schemaSource = remember(context.applicationContext) {
        val assets = context.applicationContext.assets
        SingBoxEditorSchema {
            assets.open("sing-box.schema.json").bufferedReader().use {
                singBoxSchemaJson.parseToJsonElement(it.readText()).jsonObject
            }
        }
    }
    val schema = remember(schemaSource, documentId) { schemaSource.forDocument(documentId) }
    val schemaText = remember(schemaContext) { SchemaText { id, args -> schemaContext.getString(id, *args) } }
    val schemaValidator = remember(schema, schemaText) { SingBoxSchemaValidator(schema, schemaText) }
    val completionProvider = remember(schema, schemaText) {
        SingBoxSchemaCompletionProvider(schema, schemaText)
    }
    val coroutineScope = rememberCoroutineScope()
    var hasLoaded by remember(documentId, controller) {
        mutableStateOf(controller.getText().isNotEmpty() || controller.isModified)
    }
    // 重建页面后保留草稿的版本，不能用重新读取的版本替旧草稿通过冲突检查。
    var documentRevision by rememberSaveable(documentId) { mutableStateOf("") }
    var isSaving by remember { mutableStateOf(false) }
    var formatJob by remember(documentId) { mutableStateOf<Job?>(null) }
    DisposableEffect(documentId) {
        onDispose { formatJob?.cancel() }
    }
    var showBackendConfirmation by remember { mutableStateOf(false) }
    var confirmedBackendSwitch by remember { mutableStateOf(false) }
    var errorText by remember { mutableStateOf("") }
    var schemaIssues by remember { mutableStateOf(emptyList<SingBoxSchemaIssue>()) }
    var showSchemaIssuesDialog by rememberSaveable(documentId) { mutableStateOf(false) }
    var validationState by remember { mutableStateOf(EditorValidationState.Checking) }
    var showDiscardDialog by rememberSaveable { mutableStateOf(false) }
    var showSaveFailureDialog by rememberSaveable { mutableStateOf(false) }
    var saveFailureDetail by rememberSaveable(documentId) { mutableStateOf("") }
    var saveErrorText by remember(documentId) { mutableStateOf("") }
    var softWrap by rememberSaveable { mutableStateOf(false) }
    var isExiting by remember { mutableStateOf(false) }

    val document = configState.documents.firstOrNull { it.id == documentId }
    val displayFilename = document?.let { documentTitle(it) } ?: documentId.substringAfterLast('/')
    val isEditable = document?.editable ?: !documentId.startsWith("runtime/")
    val usesRootSchema = document?.category != SingBoxDocumentCategory.LocalRule &&
            !documentId.startsWith("singbox/rules/local/")

    LaunchedEffect(documentId) {
        if (configState.documents.isEmpty()) viewModel.refreshDocuments()
        viewModel.openDocument(documentId)
    }

    LaunchedEffect(
        configState.activeDocumentId,
        configState.activeDocumentContent,
        configState.isLoadingDocument,
        configState.documentLoadError,
        documentId,
    ) {
        if (hasLoaded) return@LaunchedEffect
        if (configState.activeDocumentId != documentId || configState.isLoadingDocument ||
            configState.documentLoadError
        ) return@LaunchedEffect
        controller.setDocument(configState.activeDocumentContent)
        documentRevision = configState.activeDocumentRevision
        hasLoaded = true
    }

    val formatError = stringResource(R.string.json_format_error)
    val savedMessage = stringResource(R.string.json_saved)
    val canSave = isEditable && hasLoaded && documentRevision.isNotEmpty() && controller.isModified &&
            !controller.isComposing && !isSaving && !showBackendConfirmation

    val documentVersion = controller.documentVersion
    LaunchedEffect(documentVersion) {
        formatJob?.cancel()
        saveErrorText = ""
        saveFailureDetail = ""
        showSaveFailureDialog = false
    }
    LaunchedEffect(documentVersion, hasLoaded, isSaving, usesRootSchema, schemaValidator) {
        errorText = ""
        schemaIssues = emptyList()
        showSchemaIssuesDialog = false
        validationState = EditorValidationState.Checking
        if (!hasLoaded || isSaving) return@LaunchedEffect

        delay(VALIDATION_DEBOUNCE_MS)
        val text = controller.getText()
        val result = validateEditorJson(text, schemaValidator.takeIf { usesRootSchema }, schemaText)
        ensureActive()
        if (controller.documentVersion != documentVersion) return@LaunchedEffect
        when (result) {
            SingBoxSchemaValidationResult.Valid -> {
                validationState = EditorValidationState.Valid
            }

            is SingBoxSchemaValidationResult.Invalid -> {
                schemaIssues = result.issues
                errorText = resources.getString(
                    R.string.json_schema_invalid,
                    result.issues.size,
                )
                validationState = EditorValidationState.Invalid
            }

            is SingBoxSchemaValidationResult.Unavailable -> {
                errorText = resources.getString(
                    R.string.json_schema_unavailable,
                    result.reason,
                )
                validationState = EditorValidationState.Unavailable
            }
        }
    }

    fun formatDocument() {
        formatJob?.cancel()
        val version = controller.documentVersion
        val text = controller.getText()
        formatJob = coroutineScope.launch {
            val formatted = try {
                formatEditorJson(parseEditorJson(text))
            } catch (error: CancellationException) {
                throw error
            } catch (_: IllegalArgumentException) {
                if (controller.documentVersion == version) errorText = formatError
                return@launch
            }
            ensureActive()
            if (controller.documentVersion != version || controller.isComposing || isSaving) return@launch
            controller.replaceRange(
                TextPosition(0, 0),
                TextPosition(Int.MAX_VALUE, Int.MAX_VALUE),
                formatted,
            )
        }
    }

    fun saveDocument() {
        val confirmed = confirmedBackendSwitch
        confirmedBackendSwitch = false
        val version = controller.documentVersion
        val expectedRevision = documentRevision
        val text = controller.getText(controller.lineEnding)
        formatJob?.cancel()
        saveErrorText = ""
        saveFailureDetail = ""
        showSaveFailureDialog = false
        isSaving = true

        val onComplete: (SingBoxDocumentSaveResult) -> Unit = { result ->
            isSaving = false
            if (result.confirmationRequired) {
                showBackendConfirmation = true
            } else if (result.success) {
                documentRevision = result.revision
                controller.markSaved(version)
                coroutineScope.launch {
                    snackbarHostState.showSnackbar(savedMessage)
                }
            } else {
                saveErrorText = if (result.errorCode == "config.conflict") {
                    resources.getString(R.string.inbound_conflict)
                } else if (result.errorCode == "inbound.not_confirmed") {
                    resources.getString(R.string.inbound_apply_unconfirmed)
                } else if (result.errorCode == "inbound.restart_required") {
                    resources.getString(R.string.inbound_restart_required)
                } else if (result.restored) {
                    resources.getString(R.string.json_save_rolled_back)
                } else {
                    resources.getString(R.string.json_save_failed_summary)
                }
                saveFailureDetail = listOfNotNull(
                    saveErrorText,
                    result.errorMessage?.takeIf(String::isNotBlank),
                ).joinToString("\n\n")
                showSaveFailureDialog = true
            }
        }

        coroutineScope.launch {
            var submitted = false
            try {
                val parsed = try {
                    parseEditorJson(text)
                } catch (error: CancellationException) {
                    throw error
                } catch (error: IllegalArgumentException) {
                    errorText = error.message ?: formatError
                    validationState = EditorValidationState.Invalid
                    return@launch
                }
                val validationResult = if (usesRootSchema) schemaValidator.validate(text, parsed)
                    else SingBoxSchemaValidationResult.Valid
                ensureActive()
                if (controller.documentVersion != version) return@launch
                when (val result = validationResult) {
                    SingBoxSchemaValidationResult.Valid -> {
                        val formatted = formatEditorJson(parsed)
                        ensureActive()
                        if (controller.documentVersion != version) return@launch
                        viewModel.saveDocument(
                            documentId,
                            formatted,
                            expectedRevision,
                            confirmBackendSwitch = confirmed,
                            onComplete,
                        )
                        submitted = true
                    }

                    is SingBoxSchemaValidationResult.Invalid -> {
                        schemaIssues = result.issues
                        errorText = resources.getString(
                            R.string.json_schema_invalid,
                            result.issues.size,
                        )
                        validationState = EditorValidationState.Invalid
                    }

                    is SingBoxSchemaValidationResult.Unavailable -> {
                        errorText = resources.getString(
                            R.string.json_schema_unavailable,
                            result.reason,
                        )
                        validationState = EditorValidationState.Unavailable
                    }
                }
            } finally {
                if (!submitted) isSaving = false
            }
        }
    }

    fun exitEditor() {
        isExiting = true
    }

    fun requestBack() {
        when {
            isSaving || isExiting -> Unit
            controller.isModified -> showDiscardDialog = true
            else -> exitEditor()
        }
    }

    LaunchedEffect(isExiting) {
        if (isExiting) onBack()
    }

    BackHandler(
        enabled = !isSaving && !isExiting,
        onBack = ::requestBack,
    )

    val backdrop = rememberBlurBackdrop()
    val barColor = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface
    val editorScroll = remember(documentId) { mutableFloatStateOf(0f) }
    val editorContentOffset = remember(editorScroll) { { -editorScroll.floatValue } }
    val editorPalette = if (isInDarkTheme()) EditorColors.Default else EditorColors.Light
    // The line-number gutter masks horizontal scroll, but must not stripe the title's backdrop.
    val editorColors = remember(editorPalette) { editorPalette.copy(gutterBackground = editorPalette.background) }
    val layoutDirection = LocalLayoutDirection.current
    val validationMessage = when {
        saveErrorText.isNotBlank() -> saveErrorText
        validationState == EditorValidationState.Invalid ||
            validationState == EditorValidationState.Unavailable -> errorText
        else -> ""
    }

    val bottomInsets = WindowInsets.navigationBars.union(WindowInsets.captionBar)
        .union(WindowInsets.ime).only(WindowInsetsSides.Bottom)
    var toolbarHeight by remember { mutableIntStateOf(0) }
    val toolbarInset = with(LocalDensity.current) { toolbarHeight.toDp() }

    Scaffold(
        topBar = {
            BlurredBar(backdrop) {
                SmallTopAppBar(
                    title = displayFilename,
                    modifier = Modifier.pointerInput(Unit) { detectTapGestures { } },
                    contentOffset = editorContentOffset,
                    subtitle = if (hasLoaded && !isEditable) stringResource(R.string.json_editor_read_only) else "",
                    color = barColor,
                    navigationIcon = { BackIconButton(onClick = ::requestBack) },
                    actions = {
                        if (validationMessage.isNotBlank()) {
                            IconButton(onClick = {
                                if (saveFailureDetail.isNotBlank()) showSaveFailureDialog = true
                                else showSchemaIssuesDialog = true
                            }) {
                                Icon(chromeIcon(Icons.Rounded.ErrorOutline), validationMessage, tint = MiuixTheme.colorScheme.error)
                            }
                        }
                        IconButton(enabled = canSave, onClick = ::saveDocument) {
                            if (isSaving) {
                                CircularProgressIndicator(size = 20.dp, strokeWidth = 2.dp)
                            } else {
                                Icon(
                                    imageVector = chromeIcon(MiuixIcons.Ok),
                                    contentDescription = stringResource(R.string.save_text),
                                    tint = if (canSave) MiuixTheme.colorScheme.onSurface
                                    else MiuixTheme.colorScheme.disabledOnSecondaryVariant,
                                )
                            }
                        }
                    },
                )
            }
        },
        contentWindowInsets = WindowInsets.systemBars.add(WindowInsets.displayCutout)
            .only(WindowInsetsSides.Horizontal),
    ) { innerPadding ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(editorColors.background)
                .consumeWindowInsets(innerPadding)
                .padding(
                    start = innerPadding.calculateStartPadding(layoutDirection),
                    end = innerPadding.calculateEndPadding(layoutDirection),
                )
                .windowInsetsPadding(bottomInsets),
        ) {
            // Capture the editor only, never the toolbar that samples it.
            Box(Modifier.matchParentSize().then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)) {
                CompositionLocalProvider(
                    LocalOverscrollFactory provides null,
                    LocalEditorBottomContentInset provides toolbarInset,
                ) {
                    CodeEditor(
                        controller = controller,
                        language = EditorLanguage.PlainText,
                        colors = editorColors,
                        readOnly = !isEditable || !hasLoaded || isSaving || showBackendConfirmation,
                        softWrap = softWrap,
                        symbols = emptyList(),
                        completionProvider = completionProvider.takeIf {
                            usesRootSchema && !isExiting
                        },
                        highlighter = highlighter,
                        topContentInset = innerPadding.calculateTopPadding(),
                        onTopContentScroll = { editorScroll.floatValue = it },
                        modifier = Modifier.fillMaxSize(),
                    )
                }

                if (configState.documentLoadError) {
                    Text(
                        stringResource(R.string.json_load_failed),
                        color = MiuixTheme.colorScheme.error,
                        modifier = Modifier.align(Alignment.Center).padding(16.dp),
                    )
                } else if (!hasLoaded) {
                    CircularProgressIndicator(
                        modifier = Modifier.align(Alignment.Center).size(28.dp),
                        strokeWidth = 3.dp,
                    )
                }
            }

            val toolbarEnabled = hasLoaded && !isSaving && !showBackendConfirmation
            EditorActionBar(
                backdrop = backdrop,
                modifier = Modifier.align(Alignment.BottomCenter)
                    .onSizeChanged { toolbarHeight = it.height },
                actions = listOf(
                    EditorAction(
                        label = stringResource(R.string.json_undo),
                        enabled = toolbarEnabled && isEditable && controller.canUndo,
                        onClick = controller::undo,
                    ),
                    EditorAction(
                        label = stringResource(R.string.json_redo),
                        enabled = toolbarEnabled && isEditable && controller.canRedo,
                        onClick = controller::redo,
                    ),
                    EditorAction(
                        label = stringResource(R.string.json_find),
                        enabled = toolbarEnabled,
                        onClick = controller::openFind,
                    ),
                    EditorAction(
                        label = stringResource(R.string.json_replace_short),
                        enabled = toolbarEnabled && isEditable,
                        onClick = controller::openReplace,
                    ),
                    EditorAction(
                        label = stringResource(R.string.json_goto_line_short),
                        enabled = toolbarEnabled,
                        onClick = controller::openGotoLine,
                    ),
                    EditorAction(
                        label = stringResource(R.string.json_completion_short),
                        enabled = toolbarEnabled && isEditable && usesRootSchema,
                        onClick = controller::openCompletion,
                    ),
                    EditorAction(
                        label = stringResource(R.string.json_format_short),
                        enabled = toolbarEnabled && isEditable,
                        onClick = ::formatDocument,
                    ),
                    EditorAction(
                        label = stringResource(R.string.json_soft_wrap_short),
                        enabled = toolbarEnabled,
                        selected = softWrap,
                        onClick = { softWrap = !softWrap },
                    ),
                ) + if (isEditable) JsonEditorSymbols.map { symbol ->
                    EditorAction(
                        label = symbol.label,
                        enabled = toolbarEnabled,
                        onClick = { controller.typeSymbol(symbol) },
                    )
                } else emptyList(),
            )
            AppSnackbarHost(
                snackbarHostState,
                Modifier.align(Alignment.BottomCenter).padding(bottom = toolbarInset + 8.dp),
            )
        }
    }

    OverlayDialog(
        show = showBackendConfirmation,
        title = stringResource(R.string.inbound_switch_title),
        summary = stringResource(R.string.inbound_switch_warning),
        onDismissRequest = { showBackendConfirmation = false },
    ) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            AppDialogButton(
                text = stringResource(android.R.string.cancel),
                onClick = { showBackendConfirmation = false },
                modifier = Modifier.weight(1f),
            )
            AppDialogButton(
                text = stringResource(R.string.save_text),
                onClick = {
                    showBackendConfirmation = false
                    confirmedBackendSwitch = true
                    saveDocument()
                },
                modifier = Modifier.weight(1f),
                role = DialogActionRole.Primary,
            )
        }
    }

    OverlayDialog(
        show = showSaveFailureDialog,
        title = stringResource(R.string.json_save_failed),
        summary = saveFailureDetail,
        onDismissRequest = { showSaveFailureDialog = false },
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            AppDialogButton(
                text = stringResource(R.string.json_continue_editing),
                onClick = { showSaveFailureDialog = false },
                modifier = Modifier.weight(1f),
                role = DialogActionRole.Primary,
            )
            AppDialogButton(
                text = stringResource(R.string.json_discard_and_exit),
                role = DialogActionRole.Destructive,
                onClick = {
                    showSaveFailureDialog = false
                    exitEditor()
                },
                modifier = Modifier.weight(1f),
            )
        }
    }

    OverlayDialog(
        show = showSchemaIssuesDialog,
        title = if (schemaIssues.isEmpty()) stringResource(R.string.json_schema_problem)
            else stringResource(R.string.json_schema_issues_title, schemaIssues.size),
        summary = if (schemaIssues.isEmpty()) validationMessage
            else stringResource(R.string.json_schema_issues_summary),
        onDismissRequest = { showSchemaIssuesDialog = false },
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = 420.dp)
                .verticalScroll(rememberScrollState()),
        ) {
            schemaIssues.forEach { issue ->
                val location = when {
                    issue.line != null && issue.column != null -> resources.getString(
                        R.string.json_schema_error_position,
                        issue.line,
                        issue.column,
                    )

                    issue.instancePath.isNotBlank() -> issue.instancePath
                    else -> null
                }
                Text(
                    text = buildString {
                        append(issue.message)
                        if (!location.isNullOrBlank()) append("\n").append(location)
                    },
                    color = MiuixTheme.colorScheme.error,
                    style = MiuixTheme.textStyles.body2,
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable {
                            issue.line?.let { controller.jumpToLine((it - 1).coerceAtLeast(0)) }
                            showSchemaIssuesDialog = false
                        }
                        .padding(horizontal = 4.dp, vertical = 10.dp),
                )
            }
        }
    }

    OverlayDialog(
        show = showDiscardDialog,
        title = stringResource(R.string.json_unsaved_title),
        summary = stringResource(R.string.json_unsaved_summary),
        onDismissRequest = { showDiscardDialog = false },
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            AppDialogButton(
                text = stringResource(R.string.json_continue_editing),
                onClick = { showDiscardDialog = false },
                modifier = Modifier.weight(1f),
                role = DialogActionRole.Primary,
            )
            AppDialogButton(
                text = stringResource(R.string.json_discard_changes),
                role = DialogActionRole.Destructive,
                onClick = {
                    showDiscardDialog = false
                    exitEditor()
                },
                modifier = Modifier.weight(1f),
            )
        }
    }
}

private data class EditorAction(
    val label: String,
    val enabled: Boolean,
    val selected: Boolean = false,
    val onClick: () -> Unit,
)

@Composable
private fun EditorActionBar(
    backdrop: top.yukonga.miuix.kmp.blur.LayerBackdrop?,
    actions: List<EditorAction>,
    modifier: Modifier = Modifier,
) {
    GlassFloatingToolbar(modifier = modifier.fillMaxWidth(), backdrop = backdrop) {
        Row(Modifier.horizontalScroll(rememberScrollState()).padding(8.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically) {
            actions.forEach { action ->
                top.yukonga.miuix.kmp.basic.IconButton(
                    onClick = action.onClick,
                    enabled = action.enabled,
                    modifier = Modifier.focusProperties { canFocus = false }
                        .semantics { selected = action.selected },
                ) {
                    Text(action.label, modifier = Modifier.padding(horizontal = 8.dp),
                        style = MiuixTheme.textStyles.body2,
                        color = when {
                            !action.enabled -> MiuixTheme.colorScheme.disabledOnSurface
                            action.selected -> MiuixTheme.colorScheme.primary
                            else -> MiuixTheme.colorScheme.onSurface
                        }, maxLines = 1)
                }
            }
        }
    }
}

private enum class EditorValidationState {
    Checking,
    Valid,
    Invalid,
    Unavailable,
}

private const val VALIDATION_DEBOUNCE_MS = 500L

private val JsonEditorSymbols = listOf(
    EditorSymbol("⇥", "    "),
    EditorSymbol("{"),
    EditorSymbol("}"),
    EditorSymbol("["),
    EditorSymbol("]"),
    EditorSymbol("\""),
    EditorSymbol(":"),
    EditorSymbol(","),
)
