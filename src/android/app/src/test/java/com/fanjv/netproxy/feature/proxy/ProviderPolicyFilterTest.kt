package com.fanjv.netproxy.feature.proxy

import com.fanjv.netproxy.feature.proxy.data.PolicyGroupDraft
import com.fanjv.netproxy.feature.proxy.data.PolicyMember
import com.fanjv.netproxy.feature.proxy.data.PolicyRegexConflict
import com.fanjv.netproxy.feature.proxy.data.ProviderPolicyFilter
import com.fanjv.netproxy.feature.proxy.data.addPolicyMember
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ProviderPolicyFilterTest {
    @Test fun mixedModesRoundTripEscapedFullLabels() {
        val providers = listOf("work.+", "family")
        val selected = "work.+/hello|world\\[1]"
        val filter = ProviderPolicyFilter().select("work.+", setOf(selected))
        val include = filter.encode(providers)
        assertEquals(filter, ProviderPolicyFilter.decode(include, providers))
        assertTrue(Regex(include).matches(selected))
        assertFalse(Regex(include).matches("work.+/new"))
        assertTrue(Regex(include).matches("family/new"))
        assertFalse(Regex(include).matches("work/hello"))
        assertEquals("", filter.select("work.+", null).encode(providers))
    }

    @Test fun addingProviderDoesNotModifyStaticMembershipOrExistingProviderMode() {
        val before = PolicyGroupDraft(tag = "Proxy", outbounds = listOf("direct"), providers = listOf("family"))
        val after = before.addPolicyMember(PolicyMember("work/Munich (2)", "socks", providerTag = "work"))
        assertEquals(before.outbounds, after.outbounds)
        assertEquals(listOf("family", "work"), after.providers)
        assertTrue(Regex(after.include).matches("work/Munich (2)"))
        assertFalse(Regex(after.include).matches("work/Other"))
        assertTrue(Regex(after.include).matches("family/Other"))
    }

    @Test fun customFiltersAreNotRewritten() {
        val before = PolicyGroupDraft(tag = "Proxy", providers = listOf("work"), include = "Munich|Paris")
        assertNull(ProviderPolicyFilter.decode(before.include, before.providers))
        val member = PolicyMember("work/Tokyo", "socks", providerTag = "work")
        assertTrue(runCatching { before.addPolicyMember(member) }.exceptionOrNull() is PolicyRegexConflict)
        val excluded = before.copy(include = "", exclude = "Tokyo")
        assertTrue(runCatching { excluded.addPolicyMember(member) }.exceptionOrNull() is PolicyRegexConflict)
        assertEquals(before.include, before.addPolicyMember(PolicyMember("direct", "direct")).include)
    }
}
