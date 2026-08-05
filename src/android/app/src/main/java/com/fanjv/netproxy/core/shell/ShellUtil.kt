package com.fanjv.netproxy.core.shell

import com.topjohnwu.superuser.Shell
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * 通过 libsu 提供 root shell 访问的工具。
 */
object ShellUtil {

    @Volatile
    private var isConfigured = false
    fun configure() {
        if (isConfigured) return

        synchronized(this) {
            if (isConfigured) return

            // 若其他代码路径已创建主 shell，libsu 会拒绝替换 builder；
            // 吞掉该情况以保证启动绝不崩溃。
            runCatching {
                Shell.setDefaultBuilder(
                    Shell.Builder.create()
                        .setFlags(Shell.FLAG_MOUNT_MASTER)
                        .setTimeout(10)
                )
            }

            isConfigured = true
        }
    }

    /**
     * 检查 root 是否可用；若尚未授权会触发 SU 请求。
     */
    suspend fun isRootAvailable(): Boolean = withContext(Dispatchers.IO) {
        Shell.getShell().isRoot
    }

}
