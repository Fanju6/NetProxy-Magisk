package com.fanjv.netproxy.navigation

import android.os.Parcelable
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.snapshots.SnapshotStateList
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.navigation3.runtime.NavKey
import kotlinx.parcelize.Parcelize
import kotlinx.serialization.Serializable

/**
 * 类型安全的导航键（Navigation3），以及后退栈持有者和向各屏幕暴露它的 CompositionLocal。
 */
sealed interface Route : NavKey, Parcelable {
    @Parcelize
    @Serializable
    data object Main : Route

    @Parcelize
    @Serializable
    data object Apps : Route

    @Parcelize
    @Serializable
    data class SubscriptionDetails(val id: String) : Route

    @Parcelize
    @Serializable
    data class SubscriptionEdit(val id: String) : Route

    @Parcelize
    @Serializable
    data class NodeEdit(val nodeRef: String) : Route

    @Parcelize
    @Serializable
    data object ProxySettings : Route

    @Parcelize
    @Serializable
    data object ThemeSettings : Route

    @Parcelize
    @Serializable
    data object KernelSettings : Route

    @Parcelize
    @Serializable
    data object Logs : Route

    @Parcelize
    @Serializable
    data object About : Route

    @Parcelize
    @Serializable
    data class JsonEdit(val documentId: String) : Route

}

/**
 * 持有后退栈的简单导航助手。
 */
class Navigator(
    initialKey: NavKey
) {
    val backStack: SnapshotStateList<NavKey> = mutableStateListOf(initialKey)

    fun push(key: NavKey) {
        backStack.add(key)
    }

    fun pop() {
        if (backStack.size > 1) {
            backStack.removeAt(backStack.lastIndex)
        }
    }

    companion object {
        val Saver: Saver<Navigator, Any> = listSaver(
            save = { it.backStack.toList() },
            restore = { savedList ->
                val navigator = Navigator(savedList.firstOrNull() ?: Route.Main)
                navigator.backStack.clear()
                navigator.backStack.addAll(savedList)
                navigator
            }
        )
    }
}

@Composable
fun rememberNavigator(startRoute: NavKey): Navigator {
    return rememberSaveable(startRoute, saver = Navigator.Saver) {
        Navigator(startRoute)
    }
}

val LocalNavigator = staticCompositionLocalOf<Navigator> {
    error("LocalNavigator not provided")
}
