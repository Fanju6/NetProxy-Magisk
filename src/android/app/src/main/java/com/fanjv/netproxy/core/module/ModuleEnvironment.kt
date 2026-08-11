package com.fanjv.netproxy.core.module

import android.app.ActivityManager
import android.content.Context
import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.core.shell.ShellUtil

internal data class ModuleAvailability(
    val rootGranted: Boolean,
    val moduleInstalled: Boolean
)

/** 隔离仪表盘所需的 Android 系统与 root 环境查询。 */
internal interface ModuleEnvironment {
    val totalMemoryBytes: Long
    suspend fun availability(): ModuleAvailability
}

internal class AndroidModuleEnvironment(
    context: Context,
    private val client: NetProxyCtlClient
) : ModuleEnvironment {
    override val totalMemoryBytes: Long = ActivityManager.MemoryInfo().also { info ->
        (context.getSystemService(Context.ACTIVITY_SERVICE) as? ActivityManager)
            ?.getMemoryInfo(info)
    }.totalMem

    override suspend fun availability(): ModuleAvailability {
        val root = ShellUtil.isRootAvailable()
        return ModuleAvailability(
            rootGranted = root,
            moduleInstalled = root && client.isAvailable()
        )
    }
}
