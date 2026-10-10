package com.vorsk.portquiz

import android.os.SystemClock

/**
 * The current or last scan, shared by [ScanService], which updates it, and
 * [MainActivity], which shows it. It lives as long as the app's process, so
 * results survive leaving and reopening the app. Only use it on the main
 * thread.
 */
object ScanState {
    /** One line of results: a range of ports with the same result. */
    data class Line(val text: String, val open: Boolean)

    var host = ""
        private set
    var running = false
        private set
    var done = 0L
        private set
    var total = 0L
        private set
    var open = 0L
        private set
    var startedAt = 0L
        private set
    var finishedAt = 0L
        private set
    var canceled = false
        private set
    var missed = 0L
        private set
    var error: String? = null
        private set
    val lines = ArrayList<Line>()

    /** Increases with every scan, so views can tell a new scan's lines apart. */
    var generation = 0
        private set

    private val listeners = LinkedHashSet<() -> Unit>()

    fun addListener(listener: () -> Unit) {
        listeners.add(listener)
    }

    fun removeListener(listener: () -> Unit) {
        listeners.remove(listener)
    }

    private fun changed() = listeners.toList().forEach { it() }

    fun begin(host: String) {
        generation++
        this.host = host
        running = true
        done = 0
        total = 0
        open = 0
        startedAt = SystemClock.elapsedRealtime()
        finishedAt = 0
        canceled = false
        missed = 0
        error = null
        lines.clear()
        changed()
    }

    fun addRange(kind: String, first: Long, last: Long, open: Boolean) {
        val ports = if (first == last) "$first" else "$first-$last"
        lines.add(Line("${if (open) "OPEN" else "CLOSED"} $kind $ports", open))
        changed()
    }

    fun progress(done: Long, total: Long, open: Long) {
        this.done = done
        this.total = total
        this.open = open
        changed()
    }

    fun finish(canceled: Boolean, missed: Long, error: String?) {
        running = false
        finishedAt = SystemClock.elapsedRealtime()
        this.canceled = canceled
        this.missed = missed
        this.error = error
        changed()
    }

    /** Milliseconds the scan has been running, or ran for. */
    fun elapsed(): Long =
        (if (running) SystemClock.elapsedRealtime() else finishedAt) - startedAt

    /** Estimated milliseconds left, from the average rate so far, or null. */
    fun remaining(): Long? {
        val elapsed = elapsed()
        if (!running || done == 0L || total == 0L || elapsed < 2000) return null
        return (total - done) * elapsed / done
    }
}

/** Formats milliseconds as e.g. "45s", "3m 20s" or "1h 5m". */
fun formatDuration(millis: Long): String {
    val s = (millis + 500) / 1000
    return when {
        s < 60 -> "${s}s"
        s < 3600 -> "${s / 60}m ${s % 60}s"
        else -> "${s / 3600}h ${s % 3600 / 60}m"
    }
}
