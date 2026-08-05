package top.yukonga.scripta.editor

/**
 * 简单有界 LRU：get 命中与 set 写入都把键移到「最近使用」端，超容量时淘汰最久未用的键。
 * commonMain 没有 java.util 的 accessOrder LinkedHashMap / removeEldestEntry，故手动维护顺序。
 *
 * 用侵入式双链表 + HashMap（而非 LinkedHashMap 的 remove+重插）：get / set 命中只改指针、不分配节点——
 * 这条缓存在 draw 每帧每可见行都被调（layout / 行号），LinkedHashMap 每次命中重插都新建一个 Entry 节点，
 * 稳态滚动下白白堆垃圾。侵入式链表把命中路径的分配降到零，只有真正的插入（miss）才新建一个 [Node]。
 *
 * 相比更早的「超上限即整表 clear()」——那会连正在显示的行一起丢弃、下一帧整屏重测——LRU 保证刚访问过的
 * 可见窗口永不被一次性淘汰，消除周期性顿挫。
 */
internal class LruCache<K, V>(private val maxSize: Int) {
    private class Node<K, V>(val key: K, var value: V) {
        var prev: Node<K, V>? = null
        var next: Node<K, V>? = null
    }

    private val map = HashMap<K, Node<K, V>>()
    private var head: Node<K, V>? = null // 最久未用（淘汰端）
    private var tail: Node<K, V>? = null // 最近使用

    private fun unlink(n: Node<K, V>) {
        val p = n.prev
        val nx = n.next
        if (p != null) p.next = nx else head = nx
        if (nx != null) nx.prev = p else tail = p
        n.prev = null
        n.next = null
    }

    private fun appendToTail(n: Node<K, V>) {
        n.prev = tail
        n.next = null
        val t = tail
        if (t != null) t.next = n else head = n
        tail = n
    }

    private fun touch(n: Node<K, V>) {
        if (tail === n) return // 已是最近使用，免动
        unlink(n)
        appendToTail(n)
    }

    operator fun get(key: K): V? {
        val n = map[key] ?: return null
        touch(n) // 仅指针移动，不分配
        return n.value
    }

    operator fun set(key: K, value: V) {
        val existing = map[key]
        if (existing != null) {
            existing.value = value
            touch(existing)
            return
        }
        val n = Node(key, value)
        map[key] = n
        appendToTail(n)
        if (map.size > maxSize) {
            head?.let { evict ->
                unlink(evict) // 淘汰队首 = 最久未用
                map.remove(evict.key)
            }
        }
    }

    fun getOrPut(key: K, defaultValue: () -> V): V {
        get(key)?.let { return it }
        val v = defaultValue()
        set(key, v)
        return v
    }

    val size: Int get() = map.size
}
