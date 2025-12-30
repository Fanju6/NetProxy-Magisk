package www.netproxy.web.ui

import android.app.Application
import android.os.Handler
import android.os.Looper
import com.topjohnwu.superuser.Shell
import java.util.concurrent.Executors

class App : Application() {
    companion object {
        lateinit var instance: App
            private set
        val executor by lazy { Executors.newCachedThreadPool() }
        val handler = Handler(Looper.getMainLooper())
    }

    override fun onCreate() {
        super.onCreate()
        instance = this
        Shell.setDefaultBuilder(Shell.Builder.create().setFlags(Shell.FLAG_MOUNT_MASTER))
    }
}
