package com.fanjv.netproxy.feature.about.presentation

import android.text.Html
import android.text.Spanned
import android.text.style.URLSpan

internal fun extractLinks(html: String): List<AboutLink> = buildList {
    html.split("<br/>", "<br>", "\n").forEach { line ->
        val text: Spanned = Html.fromHtml(line, Html.FROM_HTML_MODE_LEGACY)
        val label = text.toString().trim()
        text.getSpans(0, text.length, URLSpan::class.java).forEach { span ->
            add(AboutLink(label = label, url = span.url))
        }
    }
}
