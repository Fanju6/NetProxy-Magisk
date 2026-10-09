package com.fanjv.netproxy.core.command

import com.topjohnwu.superuser.Shell
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlin.coroutines.resume

/** 安全构造 root 命令；业务操作只允许调用 netproxyctl 或 Android 的 pm。 */
internal object ShellCommand {
    suspend fun execute(vararg args: String, isolated: Boolean = false): Shell.Result {
        val command = args.joinToString(" ", transform = ::quote)
        if (isolated) return withContext(Dispatchers.IO) {
            Shell.Builder.create().setFlags(Shell.FLAG_MOUNT_MASTER).setTimeout(10).build().use { shell ->
                check(shell.isRoot) { "未获得 Root 权限" }
                await(shell.newJob().add(command).to(ArrayList(), ArrayList()))
            }
        }
        return await(Shell.cmd(command))
    }

    private suspend fun await(job: Shell.Job): Shell.Result = suspendCancellableCoroutine { continuation ->
        job.submit(null) { result -> continuation.resume(result) }
    }

    private fun quote(value: String): String =
        "'" + value.replace("'", "'\"'\"'") + "'"
}
