package com.vorsk.portquiz

import org.json.JSONObject

/** A port's result, and for a closed port how it was closed. */
enum class PortStatus(val key: String) {
    OPEN("open"),
    REJECTED("rejected"),
    DROPPED("dropped"),
    INTERFERED("interfered"),
    ERROR("error");

    val open get() = this == OPEN

    companion object {
        fun of(key: String): PortStatus = entries.firstOrNull { it.key == key } ?: ERROR
    }
}

/** A run of consecutive ports, with the same status where that applies. */
data class PortRange(val first: Int, val last: Int, val status: PortStatus) {
    val size get() = last - first + 1
}

/** How quickly open ports answered, in milliseconds. */
data class ResponseTimes(val min: Double, val median: Double, val p95: Double, val max: Double)

/** The results of one kind, such as "tcp" or "udp6". */
data class KindReport(
    val kind: String,
    val address: String,
    val tested: Int,
    val untested: Int,
    val open: Int,
    val rejected: Int,
    val dropped: Int,
    val interfered: Int,
    val errors: Int,
    val late: Int,
    val retries: Int,
    val timeoutMillis: Double,
    val responseTimes: ResponseTimes?,
    val ranges: List<PortRange>,
) {
    /** "tcp" or "udp". */
    val protocol get() = kind.take(3)

    /** "4", "6", or "" when only one IP version was tested. */
    val ipVersion get() = kind.drop(3)

    val closed get() = tested - open

    fun count(status: PortStatus): Int = when (status) {
        PortStatus.OPEN -> open
        PortStatus.REJECTED -> rejected
        PortStatus.DROPPED -> dropped
        PortStatus.INTERFERED -> interfered
        PortStatus.ERROR -> errors
    }

    /** The status of a port, or null if it was not tested. */
    fun statusOf(port: Int): PortStatus? = rangeAt(port)?.status

    /** The range containing a port, or null if it was not tested. */
    fun rangeAt(port: Int): PortRange? {
        var lo = 0
        var hi = ranges.size - 1
        while (lo <= hi) {
            val mid = (lo + hi) / 2
            val r = ranges[mid]
            when {
                port < r.first -> hi = mid - 1
                port > r.last -> lo = mid + 1
                else -> return r
            }
        }
        return null
    }

    /**
     * Runs of consecutive closed ports, however they were closed, largest
     * first. Each has the status of its ports if they all share one, else
     * null.
     */
    fun largestClosed(limit: Int): List<Pair<PortRange, PortStatus?>> {
        val runs = ArrayList<Pair<PortRange, PortStatus?>>()
        for (r in ranges) {
            if (r.status.open) continue
            val prev = runs.lastOrNull()
            if (prev != null && prev.first.last + 1 == r.first) {
                val status = if (prev.second == r.status) r.status else null
                runs[runs.size - 1] = PortRange(prev.first.first, r.last, prev.first.status) to status
            } else {
                runs.add(r to r.status)
            }
        }
        return runs.sortedWith(compareByDescending<Pair<PortRange, PortStatus?>> { it.first.size }
            .thenBy { it.first.first }).take(limit)
    }
}

/** Where IPv4 and IPv6 results differ: ports open over only one version. */
data class VersionDifference(val protocol: String, val range: PortRange, val openOverIpv6: Boolean)

/** A well-known service, checked in the summary. */
data class Service(val name: String, val port: Int, val protocol: String)

/** A scan's report, as the Go library writes it, and what the summary derives from it. */
data class ScanReport(
    val host: String,
    val seconds: Double,
    val perSecond: Double,
    val canceled: Boolean,
    val timeoutMillis: Long,
    val adaptive: Boolean,
    val kinds: List<KindReport>,
    /** The report as JSON, for saving and sharing. */
    val json: String,
) {
    val tested get() = kinds.sumOf { it.tested }
    val untested get() = kinds.sumOf { it.untested }

    /**
     * For each protocol tested over both IPv4 and IPv6, the ports open over
     * one and not the other, as ranges. Ports without a result for both are
     * skipped.
     */
    fun versionDifferences(): Map<String, List<VersionDifference>> {
        val result = LinkedHashMap<String, List<VersionDifference>>()
        for (protocol in listOf("tcp", "udp")) {
            val v4 = kinds.firstOrNull { it.kind == protocol + "4" } ?: continue
            val v6 = kinds.firstOrNull { it.kind == protocol + "6" } ?: continue
            val diffs = ArrayList<VersionDifference>()
            // walk the ports where either kind's ranges start or end
            for (r in v4.ranges) {
                var port = r.first
                while (port <= r.last) {
                    val other = v6.rangeAt(port)
                    if (other == null) {
                        port++
                        continue
                    }
                    val end = minOf(r.last, other.last)
                    if (r.status.open != other.status.open) {
                        val prev = diffs.lastOrNull()
                        if (prev != null && prev.range.last + 1 == port && prev.openOverIpv6 == other.status.open) {
                            diffs[diffs.size - 1] = prev.copy(range = prev.range.copy(last = end))
                        } else {
                            diffs.add(VersionDifference(protocol, PortRange(port, end, other.status), other.status.open))
                        }
                    }
                    port = end + 1
                }
            }
            result[protocol] = diffs
        }
        return result
    }

    companion object {
        /** Services people often need, checked in the summary. */
        val SERVICES = listOf(
            Service("FTP", 21, "tcp"),
            Service("SSH", 22, "tcp"),
            Service("Telnet", 23, "tcp"),
            Service("SMTP", 25, "tcp"),
            Service("DNS", 53, "tcp"),
            Service("DNS", 53, "udp"),
            Service("HTTP", 80, "tcp"),
            Service("POP3", 110, "tcp"),
            Service("NTP", 123, "udp"),
            Service("IMAP", 143, "tcp"),
            Service("HTTPS", 443, "tcp"),
            Service("HTTP/3 (QUIC)", 443, "udp"),
            Service("SMB", 445, "tcp"),
            Service("SMTPS", 465, "tcp"),
            Service("IPsec IKE", 500, "udp"),
            Service("SMTP submission", 587, "tcp"),
            Service("DNS over TLS", 853, "tcp"),
            Service("IMAPS", 993, "tcp"),
            Service("POP3S", 995, "tcp"),
            Service("OpenVPN", 1194, "tcp"),
            Service("OpenVPN", 1194, "udp"),
            Service("L2TP", 1701, "udp"),
            Service("PPTP", 1723, "tcp"),
            Service("MySQL", 3306, "tcp"),
            Service("RDP", 3389, "tcp"),
            Service("STUN/TURN", 3478, "udp"),
            Service("IPsec NAT-T", 4500, "udp"),
            Service("SIP", 5060, "tcp"),
            Service("SIP", 5060, "udp"),
            Service("PostgreSQL", 5432, "tcp"),
            Service("HTTP alt", 8080, "tcp"),
            Service("HTTPS alt", 8443, "tcp"),
            Service("WireGuard", 51820, "udp"),
        )

        /** Parses the JSON report written by the Go library. */
        fun parse(json: String): ScanReport {
            val o = JSONObject(json)
            val options = o.optJSONObject("options") ?: JSONObject()
            val kinds = o.optJSONArray("kinds")
            return ScanReport(
                host = o.optString("host"),
                seconds = o.optDouble("duration_seconds", 0.0),
                perSecond = o.optDouble("tested_per_second", 0.0),
                canceled = o.optBoolean("canceled"),
                timeoutMillis = options.optLong("timeout_ms"),
                adaptive = options.optBoolean("adaptive"),
                kinds = (0 until (kinds?.length() ?: 0)).map { parseKind(kinds!!.getJSONObject(it)) },
                json = json,
            )
        }

        private fun parseKind(k: JSONObject): KindReport {
            val rtt = k.optJSONObject("open_rtt_ms")
            val ranges = k.optJSONArray("ranges")
            return KindReport(
                kind = k.optString("kind"),
                address = k.optString("address"),
                tested = k.optInt("tested"),
                untested = k.optInt("untested"),
                open = k.optInt("open"),
                rejected = k.optInt("rejected"),
                dropped = k.optInt("dropped"),
                interfered = k.optInt("interfered"),
                errors = k.optInt("errors"),
                late = k.optInt("late"),
                retries = k.optInt("retries"),
                timeoutMillis = k.optDouble("timeout_ms", 0.0),
                responseTimes = rtt?.let {
                    ResponseTimes(it.optDouble("min"), it.optDouble("median"), it.optDouble("p95"), it.optDouble("max"))
                },
                ranges = (0 until (ranges?.length() ?: 0)).map {
                    val r = ranges!!.getJSONObject(it)
                    PortRange(r.getInt("first"), r.getInt("last"), PortStatus.of(r.getString("status")))
                },
            )
        }
    }
}

