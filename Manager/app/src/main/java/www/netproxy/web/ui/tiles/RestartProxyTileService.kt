package www.netproxy.web.ui.tiles

import android.service.quicksettings.TileService
import com.topjohnwu.superuser.Shell

class RestartProxyTileService : TileService() {
    
    override fun onClick() {
        super.onClick()
        // 先停止再启动代理
        Shell.cmd("/data/adb/modules/netproxy/scripts/core/stop.sh").exec()
        Shell.cmd("/data/adb/modules/netproxy/scripts/core/start.sh").submit()
    }
}
