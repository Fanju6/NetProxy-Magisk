package com.fanjv.netproxy.feature.about.presentation

import androidx.compose.runtime.Immutable

@Immutable
internal data class AboutUiState(
    val title: String,
    val appName: String,
    val versionName: String,
    val links: List<AboutLink>,
)

@Immutable
internal data class AboutScreenActions(
    val onBack: () -> Unit,
    val onOpenLink: (String) -> Unit,
)

@Immutable
internal data class AboutLink(
    val label: String,
    val url: String,
)
