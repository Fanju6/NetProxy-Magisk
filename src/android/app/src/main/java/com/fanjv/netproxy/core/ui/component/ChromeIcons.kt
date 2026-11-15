package com.fanjv.netproxy.core.ui.component

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.CloudSync
import androidx.compose.material.icons.rounded.Dashboard
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.FileOpen
import androidx.compose.material.icons.rounded.Link
import androidx.compose.material.icons.rounded.Router
import androidx.compose.material.icons.rounded.RssFeed
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material.icons.rounded.Share
import androidx.compose.ui.graphics.vector.ImageVector
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.icon.extended.Back
import top.yukonga.miuix.kmp.icon.extended.Delete
import top.yukonga.miuix.kmp.icon.extended.Download
import top.yukonga.miuix.kmp.icon.extended.MoreCircle
import top.yukonga.miuix.kmp.icon.extended.Ok
import top.yukonga.miuix.kmp.icon.extended.Refresh
import top.yukonga.miuix.kmp.icon.extended.Share
import top.yukonga.miuix.kmp.icon.glass.Add
import top.yukonga.miuix.kmp.icon.glass.Back
import top.yukonga.miuix.kmp.icon.glass.Cloud
import top.yukonga.miuix.kmp.icon.glass.Delete
import top.yukonga.miuix.kmp.icon.glass.Download
import top.yukonga.miuix.kmp.icon.glass.Edit
import top.yukonga.miuix.kmp.icon.glass.Error
import top.yukonga.miuix.kmp.icon.glass.GlobalVpn
import top.yukonga.miuix.kmp.icon.glass.GridView
import top.yukonga.miuix.kmp.icon.glass.FolderOutline
import top.yukonga.miuix.kmp.icon.glass.Link
import top.yukonga.miuix.kmp.icon.glass.More
import top.yukonga.miuix.kmp.icon.glass.Ok
import top.yukonga.miuix.kmp.icon.glass.Refresh
import top.yukonga.miuix.kmp.icon.glass.SettingsOutline
import top.yukonga.miuix.kmp.icon.glass.Share

internal fun chromeIcon(icon: ImageVector): ImageVector = when (icon) {
    MiuixIcons.Back -> MiuixIcons.Glass.Back
    MiuixIcons.MoreCircle -> MiuixIcons.Glass.More
    MiuixIcons.Add -> MiuixIcons.Glass.Add
    MiuixIcons.Ok, Icons.Rounded.Check -> MiuixIcons.Glass.Ok
    MiuixIcons.Refresh -> MiuixIcons.Glass.Refresh
    MiuixIcons.Download -> MiuixIcons.Glass.Download
    MiuixIcons.Share, Icons.Rounded.Share -> MiuixIcons.Glass.Share
    MiuixIcons.Delete -> MiuixIcons.Glass.Delete
    Icons.Rounded.Edit -> MiuixIcons.Glass.Edit
    Icons.Rounded.ErrorOutline -> MiuixIcons.Glass.Error
    Icons.Rounded.Dashboard -> MiuixIcons.Glass.GridView
    Icons.Rounded.Router -> MiuixIcons.Glass.GlobalVpn
    Icons.Rounded.CloudSync, Icons.Rounded.RssFeed -> MiuixIcons.Glass.Cloud
    Icons.Rounded.Link -> MiuixIcons.Glass.Link
    Icons.Rounded.FileOpen -> MiuixIcons.Glass.FolderOutline
    Icons.Rounded.Settings -> MiuixIcons.Glass.SettingsOutline
    else -> icon
}
