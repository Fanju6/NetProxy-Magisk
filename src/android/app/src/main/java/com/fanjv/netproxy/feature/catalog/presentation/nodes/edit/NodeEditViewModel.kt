package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

internal class NodeEditViewModel : ViewModel() {
    private val _draft = MutableStateFlow<NodeEditDraft?>(null)
    val draft = _draft.asStateFlow()

    fun initialize(content: String) {
        if (_draft.value == null) _draft.value = NodeEditDraft.parse(content)
    }

    fun update(transform: (NodeEditFields) -> NodeEditFields) {
        _draft.update { draft -> draft?.let { it.copy(fields = transform(it.fields)) } }
    }
}
