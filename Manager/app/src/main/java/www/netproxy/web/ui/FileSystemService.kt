package www.netproxy.web.ui

import android.content.ComponentName
import android.content.Intent
import android.content.ServiceConnection
import android.os.IBinder
import androidx.annotation.MainThread
import com.topjohnwu.superuser.Shell
import com.topjohnwu.superuser.ipc.RootService
import com.topjohnwu.superuser.nio.FileSystemManager
import java.util.concurrent.CopyOnWriteArraySet

class FileSystemService : RootService() {
    override fun onBind(intent: Intent): IBinder {
        return FileSystemManager.getService()
    }

    interface Listener {
        fun onServiceAvailable(fs: FileSystemManager)
        fun onLaunchFailed()
    }

    companion object {
        private sealed class Status {
            data object Uninitialized : Status()
            data object CheckRoot : Status()
            data class ServiceAvailable(val fs: FileSystemManager) : Status()
        }

        private var status: Status = Status.Uninitialized
        private val connection = object : ServiceConnection {
            override fun onServiceConnected(p0: ComponentName, p1: IBinder) {
                val fs = FileSystemManager.getRemote(p1)
                status = Status.ServiceAvailable(fs)
                pendingListeners.forEach { l ->
                    l.onServiceAvailable(fs)
                    pendingListeners.remove(l)
                }
            }

            override fun onServiceDisconnected(p0: ComponentName) {
                status = Status.Uninitialized
            }

        }
        private val pendingListeners = CopyOnWriteArraySet<Listener>()

        @MainThread
        fun start(listener: Listener) {
            (status as? Status.ServiceAvailable)?.let {
                listener.onServiceAvailable(it.fs)
                return
            }
            pendingListeners.add(listener)
            if (status == Status.Uninitialized) {
                checkRoot()
            }
        }

        private fun checkRoot() {
            status = Status.CheckRoot
            App.executor.submit {
                val isRoot = Shell.Builder.create().setFlags(Shell.FLAG_MOUNT_MASTER).build().use {
                    it.isRoot
                }
                App.handler.post {
                    if (isRoot) {
                        launchService()
                    } else {
                        status = Status.Uninitialized
                        pendingListeners.forEach { l ->
                            l.onLaunchFailed()
                            pendingListeners.remove(l)
                        }
                    }
                }
            }
        }

        private fun launchService() {
            bind(Intent(App.instance, FileSystemService::class.java), connection)
        }

        fun removeListener(listener: Listener) {
            pendingListeners.remove(listener)
        }
    }
}
