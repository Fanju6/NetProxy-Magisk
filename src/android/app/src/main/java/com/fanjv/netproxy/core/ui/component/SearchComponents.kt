package com.fanjv.netproxy.core.ui.component

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearOutSlowInEasing
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.expandHorizontally
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.animation.shrinkHorizontally
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.max
import androidx.compose.ui.zIndex
import androidx.navigationevent.NavigationEventInfo
import androidx.navigationevent.compose.NavigationBackHandler
import androidx.navigationevent.compose.rememberNavigationEventState
import com.fanjv.netproxy.core.ui.theme.LocalEnablePredictiveBack
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.InputField
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.basic.Search
import top.yukonga.miuix.kmp.icon.basic.SearchCleanup
import top.yukonga.miuix.kmp.theme.MiuixTheme.colorScheme
import top.yukonga.miuix.kmp.utils.overScrollVertical

internal fun Modifier.deferredTopPadding(top: () -> Dp): Modifier =
    layout { measurable, constraints ->
        val topPx = top().roundToPx().coerceAtLeast(0)
        val placeable = measurable.measure(
            constraints.copy(
                minHeight = (constraints.minHeight - topPx).coerceAtLeast(0),
                maxHeight = if (constraints.maxHeight == Constraints.Infinity) Constraints.Infinity else (constraints.maxHeight - topPx).coerceAtLeast(
                    0
                )
            )
        )
        val width = placeable.width.coerceIn(constraints.minWidth, constraints.maxWidth)
        val height =
            (placeable.height + topPx).coerceIn(constraints.minHeight, constraints.maxHeight)
        layout(width, height) {
            placeable.place(0, topPx)
        }
    }

/** 搜索状态持有者：搜索文本、展开态、结果状态及相关动画。 */
@Stable
class SearchStatus(val label: String) {
    var searchText by mutableStateOf("")
    var current by mutableStateOf(Status.COLLAPSED)

    var offsetY by mutableStateOf(0.dp)
    var resultStatus by mutableStateOf(ResultStatus.DEFAULT)

    fun isExpand() = current == Status.EXPANDED
    fun isCollapsed() = current == Status.COLLAPSED
    fun shouldExpand() = current == Status.EXPANDED || current == Status.EXPANDING
    fun shouldCollapsed() = current == Status.COLLAPSED || current == Status.COLLAPSING
    fun isAnimatingExpand() = current == Status.EXPANDING

    fun onAnimationComplete() {
        current = when (current) {
            Status.EXPANDING -> Status.EXPANDED
            Status.COLLAPSING -> {
                searchText = ""
                Status.COLLAPSED
            }

            else -> current
        }
    }

    @Composable
    fun TopAppBarAnim(
        modifier: Modifier = Modifier,
        visible: Boolean = shouldCollapsed(),
        backgroundColor: androidx.compose.ui.graphics.Color = colorScheme.surface,
        content: @Composable () -> Unit
    ) {
        val topAppBarAlpha = animateFloatAsState(
            if (visible) 1f else 0f,
            animationSpec = tween(if (visible) 550 else 0, easing = FastOutSlowInEasing),
            label = "topAppBarAlpha"
        )
        Box(modifier = modifier) {
            Box(
                modifier = Modifier
                    .matchParentSize()
                    .background(backgroundColor)
            )
            Box(
                modifier = Modifier
                    .alpha(topAppBarAlpha.value)
            ) { content() }
        }
    }

    enum class Status { EXPANDED, EXPANDING, COLLAPSED, COLLAPSING }
    enum class ResultStatus { DEFAULT, EMPTY, LOAD, SHOW }
}

@Composable
fun SearchStatus.SearchBox(
    content: @Composable () -> Unit
) {
    val searchStatus = this
    val density = LocalDensity.current
    val offsetYPx = with(density) { (searchStatus.offsetY.toPx() * 0.9).toInt() }

    Box {
        AnimatedVisibility(
            visible = searchStatus.shouldCollapsed(),
            enter = fadeIn(tween(300, easing = LinearOutSlowInEasing)) + slideInVertically(
                tween(
                    300,
                    easing = LinearOutSlowInEasing
                )
            ) { -offsetYPx },
            exit = fadeOut(tween(300, easing = LinearOutSlowInEasing)) + slideOutVertically(
                tween(
                    300,
                    easing = LinearOutSlowInEasing
                )
            ) { -offsetYPx }
        ) {
            content()
        }
    }
}

@Composable
fun SearchStatus.SearchPager(
    defaultResult: @Composable () -> Unit,
    expandBar: @Composable (SearchStatus, () -> Dp) -> Unit = { searchStatus, padding ->
        SearchBar(searchStatus, padding)
    },
    searchBarTopPadding: () -> Dp = { 12.dp },
    result: LazyListScope.() -> Unit
) {
    val searchStatus = this
    val systemBarsPadding = WindowInsets.systemBars.asPaddingValues().calculateTopPadding()
    val topPadding by animateDpAsState(
        targetValue = if (searchStatus.shouldExpand()) {
            systemBarsPadding + 5.dp
        } else {
            max(searchStatus.offsetY, 0.dp)
        },
        animationSpec = tween(300, easing = LinearOutSlowInEasing),
        label = "topPadding"
    ) {
        searchStatus.onAnimationComplete()
    }
    val surfaceAlpha by animateFloatAsState(
        if (searchStatus.shouldExpand()) 1f else 0f,
        animationSpec = tween(200, easing = FastOutSlowInEasing),
        label = "surfaceAlpha"
    )

    Column(
        modifier = Modifier
            .fillMaxSize()
            .zIndex(5f)
            .background(colorScheme.surface.copy(alpha = surfaceAlpha))
            .semantics { onClick { false } }
            .then(
                if (!searchStatus.isCollapsed()) Modifier.pointerInput(Unit) { } else Modifier
            )
    ) {
        Row(
            Modifier
                .fillMaxWidth()
                .deferredTopPadding { topPadding }
                .then(
                    if (!searchStatus.isCollapsed()) Modifier.background(colorScheme.surface)
                    else Modifier
                ),
            horizontalArrangement = Arrangement.Start,
            verticalAlignment = Alignment.CenterVertically
        ) {
            if (!searchStatus.isCollapsed()) {
                Box(
                    modifier = Modifier
                        .weight(1f)
                        .background(colorScheme.surface)
                ) {
                    expandBar(searchStatus, searchBarTopPadding)
                }
            }
            AnimatedVisibility(
                visible = searchStatus.isExpand() || searchStatus.isAnimatingExpand(),
                enter = expandHorizontally() + slideInHorizontally(initialOffsetX = { it }),
                exit = shrinkHorizontally() + slideOutHorizontally(targetOffsetX = { it })
            ) {
                Text(
                    text = stringResource(android.R.string.cancel),
                    fontWeight = FontWeight.Bold,
                    color = colorScheme.primary,
                    modifier = Modifier
                        .deferredTopPadding(searchBarTopPadding)
                        .padding(
                            start = 4.dp,
                            end = 16.dp,
                            bottom = 12.dp
                        )
                        .clickable(
                            interactionSource = null,
                            enabled = searchStatus.isExpand(),
                            indication = null
                        ) {
                            searchStatus.searchText = ""
                            searchStatus.current = SearchStatus.Status.COLLAPSING
                        }
                )
                run {
                    if (LocalEnablePredictiveBack.current) {
                        val navEventState = rememberNavigationEventState(NavigationEventInfo.None)
                        NavigationBackHandler(
                            state = navEventState,
                            isBackEnabled = true,
                            onBackCompleted = {
                                searchStatus.searchText = ""
                                searchStatus.current = SearchStatus.Status.COLLAPSING
                            }
                        )
                    } else {
                        BackHandler(enabled = true) {
                            searchStatus.searchText = ""
                            searchStatus.current = SearchStatus.Status.COLLAPSING
                        }
                    }
                }
            }
        }
        AnimatedVisibility(
            visible = searchStatus.isExpand(),
            modifier = Modifier
                .fillMaxSize()
                .zIndex(1f),
            enter = fadeIn(),
            exit = fadeOut()
        ) {
            when (searchStatus.resultStatus) {
                SearchStatus.ResultStatus.DEFAULT -> defaultResult()
                SearchStatus.ResultStatus.EMPTY -> {
                    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Text(
                            text = stringResource(com.fanjv.netproxy.R.string.no_apps_found),
                            style = top.yukonga.miuix.kmp.theme.MiuixTheme.textStyles.body2,
                            color = colorScheme.onSurfaceVariantActions
                        )
                    }
                }

                SearchStatus.ResultStatus.LOAD -> {
                    // 如需可在此添加加载指示器
                }

                SearchStatus.ResultStatus.SHOW -> LazyColumn(
                    Modifier
                        .fillMaxSize()
                        .overScrollVertical(),
                ) {
                    result()
                }
            }
        }
    }
}

@Composable
fun SearchBar(
    searchStatus: SearchStatus,
    searchBarTopPadding: () -> Dp = { 12.dp },
) {
    val focusRequester = remember { FocusRequester() }
    var expanded by rememberSaveable { mutableStateOf(false) }

    InputField(
        query = searchStatus.searchText,
        onQueryChange = { searchStatus.searchText = it },
        label = "",
        leadingIcon = {
            Icon(
                imageVector = MiuixIcons.Basic.Search,
                contentDescription = "search",
                modifier = Modifier
                    .size(44.dp)
                    .padding(start = 16.dp, end = 8.dp),
                tint = colorScheme.onSurfaceContainerHigh,
            )
        },
        trailingIcon = {
            AnimatedVisibility(
                searchStatus.searchText.isNotEmpty(),
                enter = fadeIn() + scaleIn(),
                exit = fadeOut() + scaleOut(),
            ) {
                Icon(
                    imageVector = MiuixIcons.Basic.SearchCleanup,
                    tint = colorScheme.onSurface,
                    contentDescription = "Clean",
                    modifier = Modifier
                        .size(44.dp)
                        .padding(start = 8.dp, end = 16.dp)
                        .clickable(
                            interactionSource = null,
                            indication = null
                        ) {
                            searchStatus.searchText = ""
                        },
                )
            }
        },
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp)
            .deferredTopPadding(searchBarTopPadding)
            .padding(bottom = 12.dp)
            .focusRequester(focusRequester),
        onSearch = { },
        expanded = searchStatus.shouldExpand(),
        onExpandedChange = {
            searchStatus.current =
                if (it) SearchStatus.Status.EXPANDED else SearchStatus.Status.COLLAPSED
        }
    )
    LaunchedEffect(Unit) {
        if (!expanded && searchStatus.shouldExpand()) {
            focusRequester.requestFocus()
            expanded = true
        }
    }
}

@Composable
fun SearchBarFake(
    label: String,
    searchBarTopPadding: () -> Dp = { 12.dp },
) {
    InputField(
        query = "",
        onQueryChange = { },
        label = label,
        leadingIcon = {
            Icon(
                imageVector = MiuixIcons.Basic.Search,
                contentDescription = "search",
                modifier = Modifier
                    .size(44.dp)
                    .padding(start = 16.dp, end = 8.dp),
                tint = colorScheme.onSurfaceContainerHigh,
            )
        },
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp)
            .deferredTopPadding(searchBarTopPadding)
            .padding(bottom = 12.dp),
        onSearch = { },
        enabled = false,
        expanded = false,
        onExpandedChange = { }
    )
}
