package com.fanjv.netproxy.navigation

import org.junit.Assert.*
import org.junit.Test

class MainDestinationsTest {
    @Test fun unavailableModuleKeepsOnlyDashboardAndSettings() {
        assertEquals(listOf(AppDestination.Dashboard, AppDestination.Settings), mainDestinations(false))
        assertEquals(AppDestination.entries, mainDestinations(true))
    }

    @Test fun permissionsChangePreservesDestinationIdentity() {
        for (available in listOf(true, false, true)) {
            val destinations = mainDestinations(available)
            assertEquals(AppDestination.Settings, AppDestination.Settings.availableIn(destinations))
            assertEquals(if (available) 3 else 1, destinations.indexOf(AppDestination.Settings))
        }
        for (destination in listOf(AppDestination.Nodes, AppDestination.Subscriptions)) {
            assertEquals(AppDestination.Dashboard, destination.availableIn(mainDestinations(false)))
        }
    }
}
