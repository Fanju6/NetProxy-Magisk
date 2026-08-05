package com.fanjv.netproxy.navigation

import androidx.compose.animation.core.EaseInOut
import androidx.compose.animation.core.tween
import androidx.compose.foundation.gestures.animateScrollBy
import androidx.compose.foundation.pager.PagerState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CloudSync
import androidx.compose.material.icons.rounded.Dashboard
import androidx.compose.material.icons.rounded.Router
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.vector.ImageVector
import com.fanjv.netproxy.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlin.math.abs

@Composable
internal fun rememberMainPagerState(
    pagerState: PagerState,
    coroutineScope: CoroutineScope = rememberCoroutineScope()
): MainPagerState = rememberSaveable(
    pagerState,
    coroutineScope,
    saver = MainPagerState.saver(pagerState, coroutineScope)
) {
    MainPagerState(pagerState, coroutineScope)
}

internal class MainPagerState(
    val pagerState: PagerState,
    private val coroutineScope: CoroutineScope
) {
    var selectedPage by mutableIntStateOf(pagerState.currentPage)
        internal set

    var isNavigating by mutableStateOf(false)
        private set

    private var navigationJob: Job? = null

    fun animateToPage(targetIndex: Int) {
        if (targetIndex == selectedPage) return

        navigationJob?.cancel()
        selectedPage = targetIndex
        isNavigating = true

        val distance = abs(targetIndex - pagerState.currentPage).coerceAtLeast(2)
        val duration = 100 * distance + 100
        val layoutInfo = pagerState.layoutInfo
        val pageSize = layoutInfo.pageSize + layoutInfo.pageSpacing
        val currentDistanceInPages =
            targetIndex - pagerState.currentPage - pagerState.currentPageOffsetFraction
        val scrollPixels = currentDistanceInPages * pageSize

        navigationJob = coroutineScope.launch {
            val currentJob = coroutineContext.job
            try {
                pagerState.animateScrollBy(
                    value = scrollPixels,
                    animationSpec = tween(easing = EaseInOut, durationMillis = duration)
                )
            } finally {
                if (navigationJob == currentJob) {
                    isNavigating = false
                    if (pagerState.currentPage != targetIndex) {
                        selectedPage = pagerState.currentPage
                    }
                }
            }
        }
    }

    fun syncPage() {
        if (!isNavigating && selectedPage != pagerState.currentPage) {
            selectedPage = pagerState.currentPage
        }
    }

    companion object {
        fun saver(
            pagerState: PagerState,
            coroutineScope: CoroutineScope
        ): Saver<MainPagerState, Int> = Saver(
            save = { it.selectedPage },
            restore = { savedPage ->
                MainPagerState(pagerState, coroutineScope).apply {
                    selectedPage = savedPage
                }
            }
        )
    }
}

internal enum class AppDestination(
    val labelRes: Int,
    val icon: ImageVector,
) {
    Dashboard(R.string.dashboard, Icons.Rounded.Dashboard),
    Nodes(R.string.nodes, Icons.Rounded.Router),
    Subscriptions(R.string.subscriptions, Icons.Rounded.CloudSync),
    Settings(R.string.settings, Icons.Rounded.Settings),
}
