package com.fanjv.netproxy.core.module

/** Android 客户端唯一需要知道的模块入口。 */
internal object ModulePaths {
    const val MODULE_DIR = "/data/adb/modules/netproxy"
    const val NETPROXYCTL = "$MODULE_DIR/netproxyctl"
}
