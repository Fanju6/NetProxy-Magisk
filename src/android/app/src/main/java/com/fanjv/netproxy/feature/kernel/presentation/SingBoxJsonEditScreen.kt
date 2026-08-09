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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.R
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BackIconButton
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.JsonSyntaxHighlighter
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.core.ui.theme.LocalEnableBlur
import com.fanjv.netproxy.core.ui.theme.isInDarkTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.CircularProgressIndicator
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SmallTopAppBar
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Ok
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.scripta.editor.CodeEditor
import top.yukonga.scripta.editor.EditorColors
import top.yukonga.scripta.editor.EditorLanguage
import top.yukonga.scripta.editor.EditorSymbol
import top.yukonga.scripta.editor.SymbolBarPosition
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
    val schemaValidator = remember(context) { SingBoxSchemaValidator(context.applicationContext) }
    val completionProvider = remember(context) {
        SingBoxSchemaCompletionProvider(context.applicationContext)
    }
    val coroutineScope = rememberCoroutineScope()
    var hasLoaded by remember(documentId, controller) {
        mutableStateOf(controller.getText().isNotEmpty() || controller.isModified)
    }
    var isSaving by remember { mutableStateOf(false) }
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
    val displayFilename = document?.filename ?: documentId.substringAfterLast('/')
    val isEditable = document?.editable ?: !documentId.startsWith("runtime/")
    val usesRootSchema = document?.category != SingBoxDocumentCategory.Source &&
            !documentId.startsWith("singbox/source/")

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
        hasLoaded = true
    }

    val formatError = stringResource(R.string.json_format_error)
    val syntaxError = stringResource(R.string.json_syntax_error)
    val savedMessage = stringResource(R.string.json_saved)
    val canSave = isEditable && hasLoaded && controller.isModified &&
            !controller.isComposing && !isSaving

    val documentVersion = controller.documentVersion
    val caret = controller.caret
    val contextHelp = remember(documentVersion, caret, usesRootSchema) {
        if (hasLoaded && usesRootSchema) {
            completionProvider.contextHelp(controller.getText(), caret)
        } else {
            null
        }
    }
    LaunchedEffect(documentVersion) {
        saveErrorText = ""
        saveFailureDetail = ""
        showSaveFailureDialog = false
    }
    LaunchedEffect(documentVersion, hasLoaded, isSaving, usesRootSchema) {
        errorText = ""
        schemaIssues = emptyList()
        showSchemaIssuesDialog = false
        validationState = EditorValidationState.Checking
        if (!hasLoaded || isSaving) return@LaunchedEffect

        delay(VALIDATION_DEBOUNCE_MS)
        val text = controller.getText()
        if (runCatching { singBoxJson.parseToJsonElement(text).jsonObject }.isFailure) {
            errorText = syntaxError
            validationState = EditorValidationState.Invalid
            return@LaunchedEffect
        }
        if (!usesRootSchema) {
            validationState = EditorValidationState.Valid
            return@LaunchedEffect
        }

        when (val result = schemaValidator.validate(text)) {
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
        val formatted = runCatching {
            singBoxJsonPretty.encodeToString(singBoxJson.parseToJsonElement(controller.getText()))
        }.getOrNull()
        if (formatted == null) {
            errorText = formatError
            return
        }
        controller.replaceRange(
            TextPosition(0, 0),
            TextPosition(Int.MAX_VALUE, Int.MAX_VALUE),
            formatted,
        )
    }

    fun saveDocument() {
        val version = controller.documentVersion
        val text = controller.getText(controller.lineEnding)
        val parsed = parseJsonObjectOrError(text) { errorText = it } ?: return
        saveErrorText = ""
        saveFailureDetail = ""
        showSaveFailureDialog = false
        isSaving = true

        val onComplete: (SingBoxDocumentSaveResult) -> Unit = { result ->
            isSaving = false
            if (result.success) {
                controller.markSaved(version)
                coroutineScope.launch {
                    snackbarHostState.showSnackbar(savedMessage)
                }
            } else {
                saveErrorText = if (result.restored) {
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
            val validationResult = if (!usesRootSchema) {
                SingBoxSchemaValidationResult.Valid
            } else {
                schemaValidator.validate(text)
            }
            when (val result = validationResult) {
                SingBoxSchemaValidationResult.Valid -> {
                    viewModel.saveDocument(
                        documentId,
                        singBoxJsonPretty.encodeToString(parsed),
                        onComplete,
                    )
                }

                is SingBoxSchemaValidationResult.Invalid -> {
                    isSaving = false
                    schemaIssues = result.issues
                    errorText = resources.getString(
                        R.string.json_schema_invalid,
                        result.issues.size,
                    )
                    validationState = EditorValidationState.Invalid
                }

                is SingBoxSchemaValidationResult.Unavailable -> {
                    isSaving = false
                    errorText = resources.getString(
                        R.string.json_schema_unavailable,
                        result.reason,
                    )
                    validationState = EditorValidationState.Unavailable
                }
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

    val enableBlur = LocalEnableBlur.current
    val backdrop = rememberBlurBackdrop(enableBlur)
    val barColor = if (backdrop != null) Color.Transparent else MiuixTheme.colorScheme.surface
    val layoutDirection = LocalLayoutDirection.current

    Scaffold(
        snackbarHost = { AppSnackbarHost(snackbarHostState) },
        topBar = {
            BlurredBar(backdrop) {
                SmallTopAppBar(
                    title = displayFilename,
                    color = barColor,
                    navigationIcon = { BackIconButton(onClick = ::requestBack) },
                    actions = {
                        IconButton(enabled = canSave, onClick = ::saveDocument) {
                            if (isSaving) {
                                CircularProgressIndicator(size = 20.dp, strokeWidth = 2.dp)
                            } else {
                                Icon(
                                    imageVector = MiuixIcons.Ok,
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
        Column(
            modifier = Modifier
                .fillMaxSize()
                .then(if (backdrop != null) Modifier.layerBackdrop(backdrop) else Modifier)
                .padding(
                    start = innerPadding.calculateStartPadding(layoutDirection),
                    top = innerPadding.calculateTopPadding(),
                    end = innerPadding.calculateEndPadding(layoutDirection),
                ),
        ) {
            val statusText = when {
                configState.documentLoadError -> stringResource(R.string.json_load_failed)
                !hasLoaded -> stringResource(R.string.collecting_data)
                saveErrorText.isNotEmpty() -> saveErrorText
                validationState == EditorValidationState.Checking ->
                    stringResource(R.string.json_checking)

                validationState == EditorValidationState.Valid && !isEditable ->
                    stringResource(R.string.json_valid_read_only)

                validationState == EditorValidationState.Valid ->
                    stringResource(R.string.json_valid)

                else -> errorText
            }
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 8.dp),
            ) {
                val statusColor = if (validationState == EditorValidationState.Invalid ||
                    validationState == EditorValidationState.Unavailable ||
                    saveErrorText.isNotEmpty() ||
                    configState.documentLoadError
                ) {
                    MiuixTheme.colorScheme.error
                } else {
                    MiuixTheme.colorScheme.onSurfaceVariantSummary
                }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .then(
                            if (schemaIssues.isNotEmpty()) {
                                Modifier.clickable { showSchemaIssuesDialog = true }
                            } else {
                                Modifier
                            },
                        )
                        .padding(horizontal = 4.dp, vertical = 4.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        text = statusText,
                        color = statusColor,
                        fontSize = 13.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f),
                    )
                    if (schemaIssues.isNotEmpty()) {
                        Text(
                            text = stringResource(R.string.json_view_issues),
                            color = statusColor,
                            fontSize = 12.sp,
                            modifier = Modifier.padding(start = 12.dp),
                        )
                    }
                }
                contextHelp?.let { help ->
                    val helpText = listOfNotNull(
                        help.path,
                        help.field,
                        help.documentation,
                    ).distinct().joinToString(" · ")
                    Text(
                        text = helpText,
                        color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 2.dp),
                    )
                }
            }

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f),
            ) {
                CompositionLocalProvider(LocalOverscrollFactory provides null) {
                    CodeEditor(
                        controller = controller,
                        language = EditorLanguage.PlainText,
                        colors = if (isInDarkTheme()) EditorColors.Default else EditorColors.Light,
                        readOnly = !isEditable || !hasLoaded || isSaving,
                        softWrap = softWrap,
                        symbols = JsonEditorSymbols,
                        symbolBarPosition = SymbolBarPosition.End,
                        bottomBar = { colors ->
                            val toolbarEnabled = hasLoaded && !isSaving
                            EditorActionBar(
                                colors = colors,
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
                                ),
                            )
                        },
                        completionProvider = completionProvider.takeIf {
                            usesRootSchema && !isExiting
                        },
                        highlighter = highlighter,
                        modifier = Modifier.fillMaxSize(),
                    )
                }

                if (!hasLoaded) {
                    CircularProgressIndicator(
                        modifier = Modifier
                            .align(Alignment.Center)
                            .size(28.dp),
                        strokeWidth = 3.dp,
                    )
                }
            }
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
            TextButton(
                text = stringResource(R.string.json_continue_editing),
                onClick = { showSaveFailureDialog = false },
                modifier = Modifier.weight(1f),
            )
            TextButton(
                text = stringResource(R.string.json_discard_and_exit),
                onClick = {
                    showSaveFailureDialog = false
                    exitEditor()
                },
                modifier = Modifier.weight(1f),
                colors = ButtonDefaults.textButtonColorsPrimary(),
            )
        }
    }

    OverlayDialog(
        show = showSchemaIssuesDialog,
        title = stringResource(R.string.json_schema_issues_title, schemaIssues.size),
        summary = stringResource(R.string.json_schema_issues_summary),
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
            TextButton(
                text = stringResource(R.string.json_continue_editing),
                onClick = { showDiscardDialog = false },
                modifier = Modifier.weight(1f),
            )
            TextButton(
                text = stringResource(R.string.json_discard_changes),
                onClick = {
                    showDiscardDialog = false
                    exitEditor()
                },
                modifier = Modifier.weight(1f),
                colors = ButtonDefaults.textButtonColorsPrimary(),
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
    colors: EditorColors,
    actions: List<EditorAction>,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(horizontal = 2.dp, vertical = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(1.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        actions.forEach { action ->
            EditorActionKey(action = action, colors = colors)
        }
    }
}

@Composable
private fun EditorActionKey(
    action: EditorAction,
    colors: EditorColors,
) {
    var pressed by remember { mutableStateOf(false) }
    Box(
        modifier = Modifier
            .clip(RoundedCornerShape(4.dp))
            .background(
                when {
                    pressed -> colors.symbolBarPressed
                    action.selected -> colors.symbolBarPressed.copy(alpha = 0.72f)
                    else -> Color.Transparent
                },
            )
            .then(
                if (action.enabled) {
                    Modifier.pointerInput(action.onClick) {
                        detectTapGestures(
                            onPress = {
                                pressed = true
                                tryAwaitRelease()
                                pressed = false
                            },
                            onTap = { action.onClick() },
                        )
                    }
                } else {
                    Modifier
                },
            )
            .heightIn(min = 32.dp)
            .padding(horizontal = 8.dp, vertical = 2.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = action.label,
            color = colors.symbolBarForeground.copy(alpha = if (action.enabled) 1f else 0.38f),
            fontSize = 13.sp,
            maxLines = 1,
        )
    }
}

private val singBoxJson = Json {
    ignoreUnknownKeys = true
    isLenient = true
}

private val singBoxJsonPretty = Json(singBoxJson) {
    prettyPrint = true
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

private fun parseJsonObjectOrError(rawJson: String, onError: (String) -> Unit): JsonObject? =
    runCatching { singBoxJson.parseToJsonElement(rawJson).jsonObject }.getOrElse {
        onError(it.message ?: "Invalid JSON")
        null
    }

