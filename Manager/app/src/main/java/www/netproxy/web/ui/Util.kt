package www.netproxy.web.ui

import com.topjohnwu.superuser.Shell

inline fun <T> withNewRootShell(
    globalMnt: Boolean = false,
    block: Shell.() -> T
): T {
    return createRootShell(globalMnt).use(block)
}

fun createRootShell(globalMnt: Boolean = false): Shell {
    Shell.enableVerboseLogging = BuildConfig.DEBUG
    val builder = Shell.Builder.create()
    if (globalMnt) {
        builder.setFlags(Shell.FLAG_MOUNT_MASTER)
    }
    return builder.build()
}
