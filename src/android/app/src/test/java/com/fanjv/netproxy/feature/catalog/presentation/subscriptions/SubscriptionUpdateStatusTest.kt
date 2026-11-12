package com.fanjv.netproxy.feature.catalog.presentation.subscriptions

import com.fanjv.netproxy.feature.catalog.model.CatalogGroupSummary
import com.fanjv.netproxy.feature.catalog.model.SubscriptionProgress
import com.fanjv.netproxy.feature.catalog.model.SubscriptionUsage
import org.junit.Assert.assertEquals
import org.junit.Test

class SubscriptionUpdateStatusTest {
    @Test fun badgeDescribesUpdateStatusRatherThanNodeHealth() {
        val empty = CatalogGroupSummary(id = "fixture", name = "Fixture", type = "subscription")
        val updated = empty.copy(lastSuccessAt = "2026-01-01T00:00:00Z")
        val failed = updated.copy(lastError = "download failed")
        val expired = updated.copy(usage = SubscriptionUsage(expire = 10))

        assertEquals(SubscriptionUpdateStatus.NeverUpdated, subscriptionStatus(empty, now = 10))
        assertEquals(SubscriptionUpdateStatus.Updated, subscriptionStatus(updated, now = 10))
        assertEquals(SubscriptionUpdateStatus.Expired, subscriptionStatus(expired, now = 10))
        assertEquals(SubscriptionUpdateStatus.Failed, subscriptionStatus(failed, now = 10))
        assertEquals(
            SubscriptionUpdateStatus.Updating,
            subscriptionStatus(failed.copy(progress = SubscriptionProgress(stage = "download")), now = 10),
        )
    }
}
