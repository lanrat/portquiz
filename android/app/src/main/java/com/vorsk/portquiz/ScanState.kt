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
    /** The server addresses being tested, once the host is resolved. */
    var addresses = ""
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
    /** The scan's estimate of the milliseconds left, or -1 if none yet. */
    private var etaMillis = -1L
    /** When [etaMillis] was received. */
    private var etaAt = 0L
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
        addresses = ""
        running = true
        done = 0
        total = 0
        open = 0
        startedAt = SystemClock.elapsedRealtime()
        finishedAt = 0
        canceled = false
        missed = 0
        etaMillis = -1
        error = null
        lines.clear()
        changed()
    }

    fun resolved(addresses: String) {
        this.addresses = addresses
        changed()
    }

    /** The host, with the addresses tested when they are not the host itself. */
    fun server(): String =
        if (addresses.isEmpty() || addresses == host.removeSurrounding("[", "]")) host
        else "$host ($addresses)"

    fun addRange(kind: String, first: Long, last: Long, open: Boolean) {
        val ports = if (first == last) "$first" else "$first-$last"
        lines.add(Line("${if (open) "OPEN" else "CLOSED"} $kind $ports", open))
        changed()
    }

    fun progress(done: Long, total: Long, open: Long, etaMillis: Long) {
        this.done = done
        this.total = total
        this.open = open
        this.etaMillis = etaMillis
        etaAt = SystemClock.elapsedRealtime()
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

    /**
     * Estimated milliseconds left, or null. The scan's estimate is smoothed
     * and arrives at least every second; between updates it counts down.
     */
    fun remaining(): Long? {
        if (!running || etaMillis < 0) return null
        return (etaMillis - (SystemClock.elapsedRealtime() - etaAt)).coerceAtLeast(0)
    }
}

/**
 * Rounds an estimate of the time left for display, like the command line
 * client, so it changes in steps that match its precision: up to the next 5
 * seconds below a minute, 10 seconds below 10 minutes, and whole minutes
 * beyond.
 */
fun roundEta(millis: Long): Long = when {
    millis <= 0 -> 0
    millis < 60_000 -> (millis + 4_999) / 5_000 * 5_000
    millis < 600_000 -> (millis + 5_000) / 10_000 * 10_000
    else -> (millis + 30_000) / 60_000 * 60_000
}

/** Formats a rounded estimate, leaving out zero units: "45s", "3m 20s", "12m", "1h 5m". */
fun formatEta(millis: Long): String {
    val s = millis / 1000
    val (h, m, sec) = Triple(s / 3600, s % 3600 / 60, s % 60)
    return when {
        h > 0 -> if (m > 0) "${h}h ${m}m" else "${h}h"
        m > 0 -> if (sec > 0) "${m}m ${sec}s" else "${m}m"
        else -> "${sec}s"
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
