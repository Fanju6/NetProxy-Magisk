package com.fanjv.netproxy.navigation

import org.junit.Assert.assertEquals
import org.junit.Test
import top.yukonga.miuix.kmp.nav.core.navBackStackOf

class NavigatorTest {
    @Test fun repeatedPushDoesNotDuplicateAnyDestination() {
        for (route in listOf(Route.Traffic, Route.InboundSettings, Route.TrafficInspector("first"))) {
            val navigator = Navigator(navBackStackOf(Route.Main))
            navigator.push(route)
            navigator.push(route)

            assertEquals(listOf(Route.Main, route), navigator.backStack)
            navigator.pop()
            assertEquals(listOf(Route.Main), navigator.backStack)
        }
    }

    @Test fun equalRoutesAnywhereInStackAreIgnoredButDifferentArgumentsCanBePushed() {
        val first = Route.TrafficInspector("first")
        val second = Route.TrafficInspector("second")
        val navigator = Navigator(navBackStackOf(Route.Main))
        navigator.push(first)
        navigator.push(second)
        navigator.push(Route.TrafficInspector("first"))

        assertEquals(listOf(Route.Main, first, second), navigator.backStack)
        navigator.pop()
        navigator.pop()
        navigator.push(Route.TrafficInspector("first"))
        assertEquals(listOf(Route.Main, first), navigator.backStack)
    }
}
