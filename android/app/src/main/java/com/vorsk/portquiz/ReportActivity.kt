package com.vorsk.portquiz

import android.app.Activity
import android.content.ClipData
import android.content.Intent
import android.os.Bundle
import android.util.TypedValue
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TableLayout
import android.widget.TableRow
import android.widget.TextView
import android.widget.Toast
import java.text.NumberFormat
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * The summary of the last finished scan: totals, how closed ports were
 * closed, a map of the results, common services, the largest blocked
 * ranges and timing, with the JSON report to save or share.
 */
class ReportActivity : Activity() {
    private lateinit var report: ScanReport
    private lateinit var content: LinearLayout

    private val numbers = NumberFormat.getIntegerInstance()
    private val decimals = NumberFormat.getNumberInstance().apply { maximumFractionDigits = 1 }
    private val percent = NumberFormat.getPercentInstance().apply { maximumFractionDigits = 1 }
    private val statusNames by lazy { resources.getStringArray(R.array.status_names) }

    private val secondaryText by lazy {
        val attrs = obtainStyledAttributes(intArrayOf(android.R.attr.textColorSecondary))
        try {
            attrs.getColorStateList(0)!!
        } finally {
            attrs.recycle()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        report = ScanState.report ?: run {
            // the process was restarted, so the scan is gone
            finish()
            return
        }
        drawBehindSystemBars()
        setContentView(R.layout.activity_report)
        applyInsets(findViewById(R.id.root))
        content = findViewById(R.id.content)
        findViewById<View>(R.id.save).setOnClickListener { save() }
        findViewById<View>(R.id.share).setOnClickListener { share() }

        header()
        overview()
        portMap()
        closed()
        services()
        largestBlocked()
        timing()
    }

    private fun header() {
        text(
            getString(
                R.string.summary_header, report.host, numbers.format(report.tested),
                formatDuration((report.seconds * 1000).toLong()), decimals.format(report.perSecond),
            ),
        )
        if (report.canceled) {
            text(getString(R.string.summary_stopped, numbers.format(report.untested)), warning = true)
        }
    }

    private fun overview() {
        heading(R.string.section_overview)
        for (k in report.kinds) {
            val share = if (k.tested > 0) k.open.toDouble() / k.tested else 0.0
            text(
                getString(
                    R.string.overview_line, kindLabel(k), k.address, numbers.format(k.open),
                    numbers.format(k.tested), percent.format(share),
                ),
            )
        }
        for ((protocol, diffs) in report.versionDifferences()) {
            val name = protocol.uppercase(Locale.ROOT)
            if (diffs.isEmpty()) {
                text(getString(R.string.versions_match, name))
                continue
            }
            val ports = diffs.sumOf { it.range.size }
            text(resources.getQuantityString(R.plurals.versions_differ, ports, name, numbers.format(ports)))
            limited(diffs, 10) { d ->
                getString(
                    if (d.openOverIpv6) R.string.open_over_ipv6_only else R.string.open_over_ipv4_only,
                    portRange(d.range),
                )
            }
        }
    }

    private fun portMap() {
        // a map of a handful of ports says nothing the other sections do not
        if (report.kinds.none { it.tested >= 100 }) return
        heading(R.string.section_map)
        val inspect = TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Small)
            setTextColor(secondaryText)
            text = getString(R.string.map_hint)
        }
        for (k in report.kinds) {
            val first = k.ranges.firstOrNull()?.first ?: continue
            val last = k.ranges.last().last
            text(getString(R.string.map_ports, kindLabel(k), portRange(PortRange(first, last, PortStatus.OPEN))))
            val map = PortMapView(this).apply {
                show(k, first, last)
                contentDescription = statusCounts(k)
                onInspect = { from, to, counts ->
                    val parts = PortStatus.entries.filter { counts[it.ordinal] > 0 }.map {
                        getString(R.string.count_status, numbers.format(counts[it.ordinal]), statusNames[it.ordinal])
                    }
                    inspect.text = getString(
                        R.string.map_inspect, portRange(PortRange(from, to, PortStatus.OPEN)),
                        parts.joinToString(getString(R.string.list_separator)),
                    )
                }
            }
            content.addView(map, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
                topMargin = dp(4)
            })
            legend(k)
        }
        content.addView(inspect, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(8)
        })
    }

    /** A legend of the statuses present in a kind, with their counts. */
    private fun legend(k: KindReport) {
        val row = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL }
        for (status in PortStatus.entries) {
            val n = k.count(status)
            if (n == 0) continue
            val item = TextView(this).apply {
                setTextAppearance(android.R.style.TextAppearance_Small)
                setTextColor(secondaryText)
                text = getString(R.string.count_status, numbers.format(n), statusNames[status.ordinal])
                val swatch = getDrawable(R.drawable.swatch)!!.mutate()
                swatch.setTint(getColor(PortMapView.statusColor(status)))
                setCompoundDrawablesRelativeWithIntrinsicBounds(swatch, null, null, null)
                compoundDrawablePadding = dp(4)
            }
            row.addView(item, LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
                marginEnd = dp(12)
            })
        }
        val scroll = android.widget.HorizontalScrollView(this).apply {
            isHorizontalScrollBarEnabled = false
            addView(row)
        }
        content.addView(scroll, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(4)
            bottomMargin = dp(8)
        })
    }

    private fun closed() {
        heading(R.string.section_closed)
        for (k in report.kinds) {
            if (k.closed == 0) {
                text(getString(R.string.no_closed, kindLabel(k)))
                continue
            }
            text(getString(R.string.closed_line, kindLabel(k), statusCounts(k, open = false)))
        }
        val any = { s: PortStatus -> report.kinds.any { it.count(s) > 0 } }
        if (any(PortStatus.REJECTED)) text(getString(R.string.explain_rejected), secondary = true)
        if (any(PortStatus.DROPPED)) text(getString(R.string.explain_dropped), secondary = true)
        if (any(PortStatus.INTERFERED)) text(getString(R.string.explain_interfered), secondary = true)
        if (any(PortStatus.ERROR)) text(getString(R.string.explain_errors), secondary = true)
        for (k in report.kinds) {
            val interfered = k.ranges.filter { it.status == PortStatus.INTERFERED }
            if (interfered.isEmpty()) continue
            val shown = interfered.take(20).joinToString(getString(R.string.list_separator)) { portRange(it) }
            val more = if (interfered.size > 20) {
                getString(R.string.list_separator) + getString(R.string.and_more, numbers.format(interfered.size - 20))
            } else {
                ""
            }
            text(getString(R.string.interfered_ports, kindLabel(k), shown + more), warning = true)
        }
    }

    private fun services() {
        heading(R.string.section_services)
        val versions = report.kinds.map { it.ipVersion }.distinct()
        val table = TableLayout(this).apply { isStretchAllColumns = false; setColumnStretchable(0, true) }
        val header = TableRow(this)
        header.addView(cell(getString(R.string.service_column), bold = true))
        header.addView(cell(getString(R.string.port_column), bold = true))
        for (v in versions) {
            header.addView(
                cell(
                    getString(
                        when (v) {
                            "4" -> R.string.ipv4_column
                            "6" -> R.string.ipv6_column
                            else -> R.string.result_column
                        },
                    ),
                    bold = true,
                ),
            )
        }
        table.addView(header)
        var rows = 0
        for (service in ScanReport.SERVICES) {
            val kinds = versions.map { v -> report.kinds.firstOrNull { it.kind == service.protocol + v } }
            val statuses = kinds.map { it?.statusOf(service.port) }
            if (statuses.all { it == null }) continue
            val row = TableRow(this)
            row.addView(cell(service.name))
            row.addView(cell(getString(R.string.service_port, service.port, service.protocol)))
            for (status in statuses) {
                row.addView(
                    cell(
                        if (status == null) getString(R.string.not_tested) else statusNames[status.ordinal],
                        open = status == PortStatus.OPEN,
                    ),
                )
            }
            table.addView(row)
            rows++
        }
        if (rows == 0) {
            text(getString(R.string.services_none))
        } else {
            content.addView(table)
        }
    }

    private fun largestBlocked() {
        heading(R.string.section_largest)
        for (k in report.kinds) {
            val runs = k.largestClosed(5)
            if (runs.isEmpty()) {
                text(getString(R.string.none_blocked, kindLabel(k)))
                continue
            }
            text(kindLabel(k), bold = true)
            for ((range, status) in runs) {
                text(
                    getString(
                        R.string.largest_line, portRange(range),
                        resources.getQuantityString(R.plurals.port_count, range.size, numbers.format(range.size)),
                        if (status == null) getString(R.string.mixed) else statusNames[status.ordinal],
                    ),
                    indent = true,
                )
            }
        }
    }

    private fun timing() {
        heading(R.string.section_timing)
        text(
            getString(
                R.string.timing_overall, numbers.format(report.tested),
                formatDuration((report.seconds * 1000).toLong()), decimals.format(report.perSecond),
            ),
        )
        for (k in report.kinds) {
            val label = kindLabel(k)
            val rtt = k.responseTimes
            text(
                if (rtt == null) {
                    getString(R.string.timing_no_rtt, label)
                } else {
                    getString(
                        R.string.timing_rtt, label, decimals.format(rtt.median), decimals.format(rtt.p95),
                        decimals.format(rtt.min), decimals.format(rtt.max),
                    )
                },
            )
            text(
                if (report.adaptive) {
                    getString(
                        R.string.timing_adaptive, label, decimals.format(k.timeoutMillis),
                        numbers.format(report.timeoutMillis), k.retries,
                    )
                } else {
                    getString(R.string.timing_fixed, label, decimals.format(k.timeoutMillis), k.retries)
                },
                secondary = true,
            )
            if (k.late > 0) {
                text(resources.getQuantityString(R.plurals.timing_late, k.late, label, numbers.format(k.late)), warning = true)
            }
        }
    }

    /** A file name for the report, such as portquiz-example.com-20261013-0930.json. */
    private fun fileName(): String {
        val stamp = SimpleDateFormat("yyyyMMdd-HHmm", Locale.ROOT).format(Date())
        return getString(R.string.report_file_name, report.host.replace(Regex("[^A-Za-z0-9.-]"), "_"), stamp)
    }

    /** Lets the user choose where to save the JSON report. */
    private fun save() {
        val intent = Intent(Intent.ACTION_CREATE_DOCUMENT)
            .addCategory(Intent.CATEGORY_OPENABLE)
            .setType(ReportProvider.TYPE)
            .putExtra(Intent.EXTRA_TITLE, fileName())
        startActivityForResult(intent, SAVE_REQUEST)
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        @Suppress("DEPRECATION")
        super.onActivityResult(requestCode, resultCode, data)
        val uri = data?.data
        if (requestCode != SAVE_REQUEST || resultCode != RESULT_OK || uri == null) return
        try {
            contentResolver.openOutputStream(uri, "wt")!!.use { it.write(report.json.toByteArray()) }
            Toast.makeText(this, R.string.saved, Toast.LENGTH_SHORT).show()
        } catch (e: Exception) {
            Toast.makeText(this, getString(R.string.save_failed, e.message ?: e.toString()), Toast.LENGTH_LONG).show()
        }
    }

    /**
     * Shares the JSON report as a file. As text it could exceed what an
     * intent can carry, and Android then silently drops the share.
     */
    private fun share() {
        val uri = try {
            ReportProvider.save(this, fileName(), report.json)
        } catch (e: Exception) {
            Toast.makeText(this, getString(R.string.save_failed, e.message ?: e.toString()), Toast.LENGTH_LONG).show()
            return
        }
        val send = Intent(Intent.ACTION_SEND)
            .setType(ReportProvider.TYPE)
            .putExtra(Intent.EXTRA_STREAM, uri)
            .putExtra(Intent.EXTRA_SUBJECT, getString(R.string.notification_done_title, report.host))
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        // the chooser passes the read permission on only through ClipData
        send.clipData = ClipData.newRawUri(fileName(), uri)
        startActivity(Intent.createChooser(send, getString(R.string.share)))
    }

    /** "TCP", or "TCP over IPv4" when both IP versions might be involved. */
    private fun kindLabel(k: KindReport): String {
        val protocol = k.protocol.uppercase(Locale.ROOT)
        return if (k.ipVersion.isEmpty()) protocol else getString(R.string.kind_ip_version, protocol, k.ipVersion)
    }

    /** Counts of each status in a kind, e.g. "120 open, 400 rejected". */
    private fun statusCounts(k: KindReport, open: Boolean = true): String =
        PortStatus.entries.filter { (open || !it.open) && k.count(it) > 0 }
            .joinToString(getString(R.string.list_separator)) {
                getString(R.string.count_status, numbers.format(k.count(it)), statusNames[it.ordinal])
            }

    private fun portRange(r: PortRange): String =
        if (r.first == r.last) r.first.toString() else "${r.first}–${r.last}"

    /** Adds a line per item, up to limit, then how many more there are. */
    private fun <T> limited(items: List<T>, limit: Int, line: (T) -> String) {
        for (item in items.take(limit)) text(line(item), indent = true)
        if (items.size > limit) text(getString(R.string.and_more, numbers.format(items.size - limit)), indent = true)
    }

    private fun heading(res: Int) {
        val view = TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Medium)
            text = getString(res)
        }
        content.addView(view, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(20)
            bottomMargin = dp(4)
        })
    }

    private fun text(
        value: String,
        secondary: Boolean = false,
        warning: Boolean = false,
        bold: Boolean = false,
        indent: Boolean = false,
    ) {
        val view = TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Small)
            text = value
            when {
                warning -> setTextColor(getColor(R.color.warning))
                secondary -> setTextColor(secondaryText)
            }
            if (bold) setTypeface(typeface, android.graphics.Typeface.BOLD)
            setTextIsSelectable(true)
        }
        content.addView(view, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(4)
            if (indent) marginStart = dp(12)
        })
    }

    private fun cell(value: String, bold: Boolean = false, open: Boolean = false): TextView =
        TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Small)
            text = value
            setPadding(0, dp(2), dp(12), dp(2))
            if (bold) setTypeface(typeface, android.graphics.Typeface.BOLD)
            if (open) setTextColor(getColor(R.color.open))
        }

    private fun dp(value: Int): Int =
        TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, value.toFloat(), resources.displayMetrics).toInt()

    companion object {
        private const val SAVE_REQUEST = 1
    }
}
