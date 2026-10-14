package com.vorsk.portquiz

import org.json.JSONArray
import org.json.JSONObject

/** How a port's result compares between two runs, A and B, in the order the map draws them. */
enum class Change(val key: String) {
    OPENED("opened"),
    CLOSED("closed"),
    REASON_CHANGED("reason_changed"),
    UNCHANGED("unchanged"),
    ONLY_A("only_a"),
    ONLY_B("only_b");

    companion object {
        fun of(key: String): Change? = entries.firstOrNull { it.key == key }
    }
}

/** Ports from first to last whose status changed from a to b, as the report names statuses. */
data class StatusChange(val first: Int, val last: Int, val a: String, val b: String) {
    val size get() = last - first + 1
}

/** A run of ports with the same [Change]. */
data class ChangeRange(val first: Int, val last: Int, val change: Change)

/** The comparison of one kind, matched between the runs by protocol and IP version. */
data class KindDiff(
    /** Such as "tcp4", or just "tcp" when the IP version is not known. */
    val kind: String,
    /** The kind's name in each run, empty when it is only in the other. */
    val aKind: String,
    val bKind: String,
    val aAddress: String,
    val bAddress: String,
    val counts: Map<Change, Int>,
    val aOpen: Int,
    val bOpen: Int,
    /** The median response time of open ports in each run, in milliseconds, if any answered. */
    val aMedian: Double?,
    val bMedian: Double?,
    val changes: List<StatusChange>,
    val ranges: List<ChangeRange>,
) {
    fun count(change: Change) = counts[change] ?: 0

    val changed get() = count(Change.OPENED) + count(Change.CLOSED) + count(Change.REASON_CHANGED)
    val compared get() = changed + count(Change.UNCHANGED)

    /** "tcp" or "udp". */
    val protocol get() = kind.take(3)

    /** "4", "6", or "" when not known. */
    val ipVersion get() = kind.drop(3)
}

/**
 * Two runs compared, as the Go library's diff JSON gives it: per kind, the
 * ports whose status changed from run A to run B.
 */
data class ScanDiff(val kinds: List<KindDiff>, val json: String) {
    val changed get() = kinds.sumOf { it.changed }

    companion object {
        fun parse(json: String): ScanDiff {
            val o = JSONObject(json)
            val kinds = o.optJSONArray("kinds") ?: JSONArray()
            return ScanDiff((0 until kinds.length()).map { parseKind(kinds.getJSONObject(it)) }, json)
        }

        private fun parseKind(k: JSONObject): KindDiff {
            val changes = k.optJSONArray("changes") ?: JSONArray()
            val ranges = k.optJSONArray("ranges") ?: JSONArray()
            return KindDiff(
                kind = k.optString("kind"),
                aKind = k.optString("a_kind"),
                bKind = k.optString("b_kind"),
                aAddress = k.optString("a_address"),
                bAddress = k.optString("b_address"),
                counts = Change.entries.associateWith { k.optInt(it.key) },
                aOpen = k.optInt("a_open"),
                bOpen = k.optInt("b_open"),
                aMedian = k.optJSONObject("a_open_rtt_ms")?.optDouble("median"),
                bMedian = k.optJSONObject("b_open_rtt_ms")?.optDouble("median"),
                changes = (0 until changes.length()).map {
                    val c = changes.getJSONObject(it)
                    StatusChange(c.getInt("first"), c.getInt("last"), c.getString("a"), c.getString("b"))
                },
                ranges = (0 until ranges.length()).mapNotNull {
                    val r = ranges.getJSONObject(it)
                    Change.of(r.getString("change"))?.let { c -> ChangeRange(r.getInt("first"), r.getInt("last"), c) }
                },
            )
        }
    }
}
