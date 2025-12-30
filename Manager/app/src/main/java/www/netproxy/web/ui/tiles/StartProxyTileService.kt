package www.netproxy.web.ui.tiles

import android.service.quicksettings.TileService
import com.topjohnwu.superuser.Shell

class StartProxyTileService : TileService() {
    
    override fun onClick() {
        super.onClick()
        // 执行开启代理命令
        Shell.cmd("/data/adb/modules/netproxy/scripts/core/start.sh").submit()
    }
}
