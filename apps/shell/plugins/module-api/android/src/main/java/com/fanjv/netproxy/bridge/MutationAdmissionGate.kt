package com.fanjv.netproxy.bridge

/** WebView 与磁贴共用进程内准入，忙时拒绝，不排队重放。 */
internal class MutationAdmissionGate {
    private var owner: Lease? = null

    @Synchronized
    fun tryAcquire(): Lease? {
        if (owner != null) return null
        return Lease().also { owner = it }
    }

    inner class Lease internal constructor() : AutoCloseable {
        override fun close() = synchronized(this@MutationAdmissionGate) {
            if (owner === this) owner = null
        }
    }

    companion object {
        val process = MutationAdmissionGate()
    }
}
