package com.fanjv.netproxy.feature.apps.model

import androidx.compose.runtime.Immutable

/** Android 多用户、工作资料或应用分身所属用户。 */
@Immutable
data class UserInfo(
    val id: String,
    val name: String
)
