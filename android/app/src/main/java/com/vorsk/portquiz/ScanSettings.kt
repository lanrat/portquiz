package com.vorsk.portquiz

/** Which IP versions to test, like the client's -4 and -6 flags. */
enum class IpVersion(val ipv4: Boolean, val ipv6: Boolean) {
    /** Whichever version the server's name resolves to first. */
    AUTO(false, false),
    IPV4(true, false),
    IPV6(false, true),
    /** Every port over both versions. */
    BOTH(true, true),
}

/** Which result ranges to show. It only filters the display, not the scan. */
enum class ResultsFilter {
    ALL, OPEN, CLOSED;

    fun matches(line: ScanState.Line): Boolean = when (this) {
        ALL -> true
        OPEN -> line.open
        CLOSED -> !line.open
    }
}

/** Everything that configures a scan. */
data class ScanSettings(
    val host: String,
    val ports: String,
    val tcp: Boolean,
    val udp: Boolean,
    val password: String,
    val ipVersion: IpVersion,
    val timeoutMillis: Long,
    val adaptive: Boolean,
    val rate: Long,
    /** Retries for ports that do not respond, or -1 for the default. */
    val retries: Long,
    val parallel: Long,
)

/**
 * Parses a number typed into a settings field. A blank or invalid field
 * means [default].
 */
fun parseNumber(text: CharSequence, default: Long): Long =
    text.toString().trim().toLongOrNull()?.takeIf { it >= 0 } ?: default
