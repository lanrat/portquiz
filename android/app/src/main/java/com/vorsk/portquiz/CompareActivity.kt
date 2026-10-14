package com.vorsk.portquiz

import android.app.Activity
import android.content.ClipData
import android.content.Intent
import android.os.Bundle
import android.util.TypedValue
import android.view.View
import android.view.ViewGroup
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import com.vorsk.portquiz.mobile.Mobile
import java.text.NumberFormat
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.concurrent.thread

/**
 * Compares two past runs, A and B: per kind, how many ports opened, closed
 * or closed differently, a map of the changes and a list of them, with the
 * comparison as JSON to save or share.
 */
class CompareActivity : Activity() {
    private lateinit var history: RunHistory
    private lateinit var content: LinearLayout
    private lateinit var a: String
    private lateinit var b: String
    private var entries: Map<String, HistoryEntry> = emptyMap()
    private var reports: Map<String, String> = emptyMap()
    private var diff: ScanDiff? = null

    private val numbers = NumberFormat.getIntegerInstance()
    private val decimals = NumberFormat.getNumberInstance().apply { maximumFractionDigits = 1 }
    private val changeNames by lazy { resources.getStringArray(R.array.change_names) }

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
        a = savedInstanceState?.getString(EXTRA_A) ?: intent.getStringExtra(EXTRA_A) ?: return finish()
        b = savedInstanceState?.getString(EXTRA_B) ?: intent.getStringExtra(EXTRA_B) ?: return finish()
        drawBehindSystemBars()
        setContentView(R.layout.activity_compare)
        applyInsets(findViewById(R.id.root))
        content = findViewById(R.id.content)
        history = RunHistory.of(this)
        findViewById<View>(R.id.swap).setOnClickListener {
            a = b.also { b = a }
            compare()
        }
        findViewById<View>(R.id.save).setOnClickListener { save() }
        findViewById<View>(R.id.share).setOnClickListener { share() }

        thread(name = "portquiz-compare") {
            val list = history.list().associateBy { it.id }
            val jsons = listOf(a, b).associateWith { history.read(it) }
            runOnUiThread {
                if (isDestroyed) return@runOnUiThread
                if (jsons.values.any { it == null }) {
                    // deleted meanwhile
                    finish()
                    return@runOnUiThread
                }
                entries = list
                reports = jsons.mapValues { it.value!! }
                compare()
            }
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putString(EXTRA_A, a)
        outState.putString(EXTRA_B, b)
    }

    /** Compares A with B off the main thread, then shows the comparison. */
    private fun compare() {
        val ra = reports[a] ?: return
        val rb = reports[b] ?: return
        thread(name = "portquiz-compare") {
            val result = runCatching { ScanDiff.parse(Mobile.diff(ra, rb)) }
            runOnUiThread {
                if (isDestroyed) return@runOnUiThread
                result.onSuccess {
                    diff = it
                    render(it)
                }.onFailure {
                    content.removeAllViews()
                    text(getString(R.string.compare_failed, it.message ?: it.toString()), warning = true)
                }
            }
        }
    }

    private fun render(d: ScanDiff) {
        content.removeAllViews()
        val ea = entries[a]
        val eb = entries[b]
        if (ea != null && eb != null) {
            text(getString(R.string.compare_run_a, runTitle(this, ea), runDetail(this, ea)))
            text(getString(R.string.compare_run_b, runTitle(this, eb), runDetail(this, eb)))
            if (ea.host != eb.host) text(getString(R.string.compare_different_hosts, ea.host, eb.host), warning = true)
            if ((ea.client == "browser") != (eb.client == "browser")) text(getString(R.string.compare_browser), secondary = true)
        }
        if (d.changed == 0 && d.kinds.any { it.compared > 0 }) text(getString(R.string.compare_no_changes))
        for (k in d.kinds) kind(k)
    }

    private fun kind(k: KindDiff) {
        heading(kindName(this, k.kind))
        if (k.aKind.isEmpty() || k.bKind.isEmpty()) {
            text(getString(if (k.aKind.isEmpty()) R.string.compare_only_b else R.string.compare_only_a), secondary = true)
            return
        }
        val n = { c: Change -> numbers.format(k.count(c)) }
        val notCompared = k.count(Change.ONLY_A) + k.count(Change.ONLY_B)
        text(
            if (notCompared == 0) {
                getString(R.string.compare_counts, n(Change.OPENED), n(Change.CLOSED), n(Change.REASON_CHANGED), n(Change.UNCHANGED))
            } else {
                getString(
                    R.string.compare_counts_partial, n(Change.OPENED), n(Change.CLOSED), n(Change.REASON_CHANGED), n(Change.UNCHANGED),
                    resources.getQuantityString(R.plurals.port_count, notCompared, numbers.format(notCompared)),
                )
            },
        )
        var open = getString(R.string.compare_open, numbers.format(k.aOpen), numbers.format(k.bOpen))
        if (k.aMedian != null || k.bMedian != null) {
            val ms = { v: Double? -> if (v == null) getString(R.string.compare_none) else getString(R.string.compare_ms, decimals.format(v)) }
            open += " " + getString(R.string.compare_rtt, ms(k.aMedian), ms(k.bMedian))
        }
        text(open, secondary = true)
        if (k.aAddress != k.bAddress) text(getString(R.string.compare_addresses, k.aAddress, k.bAddress), secondary = true)
        map(k)
        for (c in k.changes.take(MAX_CHANGES)) text(changeLine(k, c), indent = true)
        if (k.changes.size > MAX_CHANGES) {
            text(getString(R.string.compare_more, numbers.format(k.changes.size - MAX_CHANGES)), secondary = true, indent = true)
        }
    }

    /** A changed range, such as "22–25 (SSH, Telnet): dropped → rejected · 4 ports". */
    private fun changeLine(k: KindDiff, c: StatusChange): String {
        var ports = if (c.first == c.last) c.first.toString() else "${c.first}–${c.last}"
        val services = ScanReport.SERVICES.filter { it.protocol == k.protocol && it.port in c.first..c.last }.map { it.name }.distinct()
        if (services.isNotEmpty()) {
            val shown = services.take(3).joinToString(getString(R.string.list_separator)) + if (services.size > 3) getString(R.string.list_separator) + "…" else ""
            ports = getString(R.string.compare_services, ports, shown)
        }
        return if (c.size == 1) getString(R.string.compare_change, ports, c.a, c.b)
        else getString(
            R.string.compare_change_size, ports, c.a, c.b,
            resources.getQuantityString(R.plurals.port_count, c.size, numbers.format(c.size)),
        )
    }

    /** The map of a kind's changes, its legend, and the line showing the ports touched. */
    private fun map(k: KindDiff) {
        val first = k.ranges.firstOrNull()?.first ?: return
        val last = k.ranges.last().last
        val inspect = TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Small)
            setTextColor(secondaryText)
            text = getString(R.string.map_hint)
        }
        val map = PortMapView(this).apply {
            show(k.ranges.map { MapRange(it.first, it.last, it.change.ordinal) }, Change.entries.map { PortMapView.changeColor(it) }, first, last)
            contentDescription = Change.entries.filter { k.count(it) > 0 }
                .joinToString(getString(R.string.list_separator)) { getString(R.string.count_status, numbers.format(k.count(it)), changeNames[it.ordinal]) }
            onInspect = { from, to, counts ->
                val parts = Change.entries.filter { counts[it.ordinal] > 0 }.map {
                    getString(R.string.count_status, numbers.format(counts[it.ordinal]), changeNames[it.ordinal])
                }
                val range = if (from == to) from.toString() else "$from–$to"
                inspect.text = getString(R.string.map_inspect, range, parts.joinToString(getString(R.string.list_separator)))
            }
        }
        content.addView(map, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(8)
        })
        val row = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL }
        for (c in Change.entries) {
            val count = k.count(c)
            if (count == 0) continue
            row.addView(TextView(this).apply {
                setTextAppearance(android.R.style.TextAppearance_Small)
                setTextColor(secondaryText)
                text = getString(R.string.count_status, numbers.format(count), changeNames[c.ordinal])
                val swatch = getDrawable(R.drawable.swatch)!!.mutate()
                swatch.setTint(getColor(PortMapView.changeColor(c)))
                setCompoundDrawablesRelativeWithIntrinsicBounds(swatch, null, null, null)
                compoundDrawablePadding = dp(4)
            }, LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
                marginEnd = dp(12)
            })
        }
        content.addView(HorizontalScrollView(this).apply {
            isHorizontalScrollBarEnabled = false
            addView(row)
        }, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(4)
        })
        content.addView(inspect)
    }

    /** A file name for the comparison, such as portquiz-compare-20261013-0930-20261014-1000.json. */
    private fun fileName(): String {
        val stamp = { id: String ->
            val millis = entries[id]?.started?.let { startedMillis(it) }?.takeIf { it > 0 }
            if (millis == null) "unknown" else SimpleDateFormat("yyyyMMdd-HHmm", Locale.ROOT).format(Date(millis))
        }
        return getString(R.string.compare_file_name, stamp(a), stamp(b))
    }

    private fun save() {
        if (diff == null) return
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
        val json = diff?.json
        if (requestCode != SAVE_REQUEST || resultCode != RESULT_OK || uri == null || json == null) return
        try {
            contentResolver.openOutputStream(uri, "wt")!!.use { it.write(json.toByteArray()) }
            Toast.makeText(this, R.string.compare_saved, Toast.LENGTH_SHORT).show()
        } catch (e: Exception) {
            Toast.makeText(this, getString(R.string.save_failed, e.message ?: e.toString()), Toast.LENGTH_LONG).show()
        }
    }

    private fun share() {
        val json = diff?.json ?: return
        val name = fileName()
        val uri = try {
            ReportProvider.save(this, name, json)
        } catch (e: Exception) {
            Toast.makeText(this, getString(R.string.save_failed, e.message ?: e.toString()), Toast.LENGTH_LONG).show()
            return
        }
        val send = Intent(Intent.ACTION_SEND)
            .setType(ReportProvider.TYPE)
            .putExtra(Intent.EXTRA_STREAM, uri)
            .putExtra(Intent.EXTRA_SUBJECT, getString(R.string.compare_title))
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        send.clipData = ClipData.newRawUri(name, uri)
        startActivity(Intent.createChooser(send, getString(R.string.share)))
    }

    private fun heading(value: String) {
        val view = TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Medium)
            text = value
        }
        content.addView(view, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(20)
            bottomMargin = dp(4)
        })
    }

    private fun text(value: String, secondary: Boolean = false, warning: Boolean = false, indent: Boolean = false) {
        val view = TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Small)
            text = value
            when {
                warning -> setTextColor(getColor(R.color.warning))
                secondary -> setTextColor(secondaryText)
            }
            setTextIsSelectable(true)
        }
        content.addView(view, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = dp(4)
            if (indent) marginStart = dp(12)
        })
    }

    private fun dp(value: Int): Int =
        TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, value.toFloat(), resources.displayMetrics).toInt()

    companion object {
        const val EXTRA_A = "a"
        const val EXTRA_B = "b"
        private const val SAVE_REQUEST = 1
        /** How many changed ranges are listed per kind; the saved JSON has all of them. */
        private const val MAX_CHANGES = 200
    }
}
