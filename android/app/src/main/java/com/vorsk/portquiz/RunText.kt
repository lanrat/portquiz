package com.vorsk.portquiz

import android.content.Context
import java.text.NumberFormat
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

// Text describing runs, shared by the list of past runs and comparisons.

/** When a run started, as a report writes it, in the phone's time zone and locale. */
fun formatStarted(started: String): String = try {
    OffsetDateTime.parse(started)
        .atZoneSameInstant(ZoneId.systemDefault())
        .format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT))
} catch (_: Exception) {
    started
}

/** When a run started, in milliseconds since the epoch, for ordering runs; 0 if unknown. */
fun startedMillis(started: String): Long = try {
    OffsetDateTime.parse(started).toInstant().toEpochMilli()
} catch (_: Exception) {
    0
}

/** A run's title: when it started, and its name if it has one. */
fun runTitle(context: Context, e: HistoryEntry): String =
    if (e.label.isEmpty()) formatStarted(e.started)
    else context.getString(R.string.run_title_label, formatStarted(e.started), e.label)

/** "TCP", or "TCP over IPv4" for a kind such as "tcp4". */
fun kindName(context: Context, kind: String): String {
    val protocol = kind.take(3).uppercase(Locale.ROOT)
    val version = kind.drop(3)
    return if (version.isEmpty()) protocol else context.getString(R.string.kind_ip_version, protocol, version)
}

/** Names the client that ran a scan, or "" if the report does not say. */
fun clientName(context: Context, e: HistoryEntry): String {
    val name = when (e.client) {
        "cli" -> context.getString(R.string.client_cli)
        "android" -> context.getString(R.string.client_android)
        "browser" -> if (e.tcpRequests.isEmpty()) context.getString(R.string.client_browser)
        else context.getString(R.string.client_browser_scheme, e.tcpRequests.uppercase(Locale.ROOT))
        else -> e.client
    }
    return if (name.isNotEmpty() && e.clientAddress.isNotEmpty()) context.getString(R.string.client_from, name, e.clientAddress) else name
}

/** A run on one line: the server, the client, what each kind found, and whether it was stopped. */
fun runDetail(context: Context, e: HistoryEntry): String {
    val numbers = NumberFormat.getIntegerInstance()
    val parts = mutableListOf(e.host, clientName(context, e))
    for (k in e.kinds) {
        parts.add(context.getString(R.string.run_kind, kindName(context, k.kind), numbers.format(k.open), numbers.format(k.tested)))
    }
    if (e.canceled) parts.add(context.getString(R.string.run_stopped))
    return parts.filter { it.isNotEmpty() }.joinToString(context.getString(R.string.run_separator))
}
