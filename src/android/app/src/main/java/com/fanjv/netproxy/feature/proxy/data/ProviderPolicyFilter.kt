package com.fanjv.netproxy.feature.proxy.data

internal class PolicyRegexConflict : IllegalArgumentException()

// Only round-trip our exact syntax. Anything else remains an advanced regex.
internal data class ProviderPolicyFilter(val members: Map<String, Set<String>> = emptyMap()) {
    fun encode(providers: List<String>): String {
        if (providers.none(members::containsKey)) return ""
        require(providers.none { '/' in it })
        val terms = providers.flatMap { provider ->
            members[provider]?.sorted()?.map(::quoteProviderLabel)
                ?: listOf(quoteProviderLabel(provider) + "/[\\s\\S]*")
        }
        return if (terms.isEmpty()) "a^" else terms.joinToString("|", "^(", ")$")
    }

    fun select(provider: String, nodes: Set<String>?) =
        copy(members = if (nodes == null) members - provider else members + (provider to nodes))

    companion object {
        fun decode(include: String, providers: List<String>): ProviderPolicyFilter? {
            if (include.isEmpty()) return ProviderPolicyFilter()
            if (providers.isEmpty() || providers.any { '/' in it }) return null
            if (include == "a^") return ProviderPolicyFilter(providers.associateWith { emptySet() })
            if (!include.startsWith("^(") || !include.endsWith(")$")) return null
            val prefixes = providers.associateWith { quoteProviderLabel(it) + "/" }
            return decodeTerms(include, providers, prefixes)
        }

        private fun decodeTerms(include: String, providers: List<String>, prefixes: Map<String, String>): ProviderPolicyFilter? {
            val raw = include.substring(2, include.length - 2)
            val terms = mutableListOf<String>()
            var escaped = false
            var start = 0
            raw.forEachIndexed { index, char ->
                if (escaped) escaped = false
                else if (char == '\\') escaped = true
                else if (char == '|') {
                    terms += raw.substring(start, index)
                    start = index + 1
                }
            }
            terms += raw.substring(start)
            val selected = providers.associateWith { mutableSetOf<String>() }.toMutableMap()
            for (term in terms) {
                val provider = prefixes.entries.singleOrNull { term.startsWith(it.value) }?.key ?: return null
                if (term == prefixes.getValue(provider) + "[\\s\\S]*") {
                    selected.remove(provider)
                    continue
                }
                val label = unquoteProviderLabel(term) ?: return null
                selected[provider]?.add(label) ?: return null
            }
            return ProviderPolicyFilter(selected).takeIf { it.encode(providers) == include }
        }
    }
}

private const val regexSpecial = "\\.+*?()|[]{}^$"

private fun quoteProviderLabel(label: String) = buildString {
    label.forEach {
        if (it in regexSpecial) append('\\')
        append(it)
    }
}

private fun unquoteProviderLabel(term: String): String? {
    val result = StringBuilder()
    var escaped = false
    for (char in term) {
        if (escaped) {
            if (char !in regexSpecial) return null
            result.append(char)
            escaped = false
        } else if (char == '\\') escaped = true
        else if (char in regexSpecial) return null
        else result.append(char)
    }
    return if (escaped) null else result.toString()
}

internal fun PolicyGroupDraft.addPolicyMember(member: PolicyMember): PolicyGroupDraft {
    val provider = member.providerTag
    if (provider.isEmpty()) {
        require(member.tag != tag)
        return copy(outbounds = (outbounds + member.tag).distinct())
    }
    if (exclude.isNotEmpty()) throw PolicyRegexConflict()
    if (useAll) {
        if (include.isEmpty()) return this
        throw PolicyRegexConflict()
    }
    val filter = ProviderPolicyFilter.decode(include, providers) ?: throw PolicyRegexConflict()
    if (provider in providers && provider !in filter.members) return this
    val next = (providers + provider).distinct()
    if (next.any { '/' in it }) throw PolicyRegexConflict()
    return copy(providers = next, include = filter.select(provider, filter.members[provider].orEmpty() + member.tag).encode(next))
}
