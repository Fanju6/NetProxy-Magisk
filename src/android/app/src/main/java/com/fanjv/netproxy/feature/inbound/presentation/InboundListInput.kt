package com.fanjv.netproxy.feature.inbound.presentation

internal data class InboundListInput(val values: List<String>) {
    val text: String get() = values.joinToString("\n")

    fun withText(text: String) = copy(values = text.lines())

    fun withSelection(value: String, selected: Boolean) = copy(
        values = if (selected) (values.filter(String::isNotEmpty) + value).distinct()
        else values.filterNot { it == value }
    )

    fun entries(): List<String> = values.filter(String::isNotEmpty).distinct()
}
