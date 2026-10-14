package com.vorsk.portquiz

import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.provider.OpenableColumns
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.PopupMenu
import android.widget.TextView
import android.widget.Toast
import com.vorsk.portquiz.mobile.Mobile
import kotlin.concurrent.thread

/**
 * The last runs, finished here or imported from any portquiz client: check
 * two to compare them, tap one for its summary, or rename or delete it.
 */
class HistoryActivity : Activity() {
    private lateinit var history: RunHistory
    private lateinit var list: LinearLayout
    private lateinit var hint: TextView
    private lateinit var compare: Button
    private lateinit var clear: Button
    private var entries: List<HistoryEntry> = emptyList()
    private val selected = LinkedHashSet<String>()

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
        drawBehindSystemBars()
        setContentView(R.layout.activity_history)
        applyInsets(findViewById(R.id.root))
        history = RunHistory.of(this)
        list = findViewById(R.id.list)
        hint = findViewById(R.id.hint)
        compare = findViewById(R.id.compare)
        clear = findViewById(R.id.clear)
        compare.setOnClickListener { compareSelected() }
        findViewById<View>(R.id.import_json).setOnClickListener { pickReports() }
        clear.setOnClickListener { confirmClear() }
        savedInstanceState?.getStringArrayList(STATE_SELECTED)?.let { selected.addAll(it) }
    }

    override fun onStart() {
        super.onStart()
        reload()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putStringArrayList(STATE_SELECTED, ArrayList(selected))
    }

    /** Reads the runs off the main thread, then shows them. */
    private fun reload() {
        background({ history.list() }) {
            entries = it
            selected.retainAll(entries.map { e -> e.id }.toSet())
            render()
        }
    }

    /** Runs work on a background thread, then done with its result on the main thread, unless the screen is gone. */
    private fun <T> background(work: () -> T, done: (T) -> Unit) {
        thread(name = "portquiz-history") {
            val result = work()
            runOnUiThread { if (!isDestroyed) done(result) }
        }
    }

    private fun render() {
        list.removeAllViews()
        for (e in entries) list.addView(row(e))
        hint.setText(if (entries.isEmpty()) R.string.history_empty else R.string.history_hint)
        clear.visibility = if (entries.isEmpty()) View.GONE else View.VISIBLE
        updateCompare()
    }

    private fun updateCompare() {
        compare.isEnabled = selected.size == 2
    }

    private fun row(e: HistoryEntry): View {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            minimumHeight = dp(56)
            background = selectableBackground()
            setOnClickListener {
                startActivity(Intent(this@HistoryActivity, ReportActivity::class.java).putExtra(ReportActivity.EXTRA_ID, e.id))
            }
        }
        val check = CheckBox(this).apply {
            isChecked = e.id in selected
            contentDescription = getString(R.string.select_run, runTitle(this@HistoryActivity, e))
            setOnCheckedChangeListener { _, checked ->
                if (checked) selected.add(e.id) else selected.remove(e.id)
                updateCompare()
            }
        }
        row.addView(check)
        val texts = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(4), dp(8), dp(4), dp(8))
        }
        texts.addView(TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Medium)
            text = runTitle(this@HistoryActivity, e)
        })
        texts.addView(TextView(this).apply {
            setTextAppearance(android.R.style.TextAppearance_Small)
            setTextColor(secondaryText)
            text = runDetail(this@HistoryActivity, e)
        })
        row.addView(texts, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        val more = TextView(this).apply {
            text = getString(R.string.more_symbol)
            contentDescription = getString(R.string.more_actions)
            setTextAppearance(android.R.style.TextAppearance_Large)
            gravity = Gravity.CENTER
            minWidth = dp(48)
            minHeight = dp(48)
            background = selectableBackground()
            setOnClickListener { v ->
                PopupMenu(this@HistoryActivity, v).apply {
                    menu.add(0, MENU_RENAME, 0, R.string.rename)
                    menu.add(0, MENU_DELETE, 1, R.string.delete)
                    setOnMenuItemClickListener {
                        when (it.itemId) {
                            MENU_RENAME -> rename(e)
                            MENU_DELETE -> background({ history.delete(e.id) }) { reload() }
                        }
                        true
                    }
                    show()
                }
            }
        }
        row.addView(more)
        return row
    }

    private fun rename(e: HistoryEntry) {
        val input = EditText(this).apply {
            setText(e.label)
            hint = getString(R.string.label_hint)
            isSingleLine = true
            setSelection(text.length)
        }
        val frame = FrameLayout(this).apply {
            setPadding(dp(20), dp(8), dp(20), 0)
            addView(input)
        }
        AlertDialog.Builder(this)
            .setTitle(R.string.rename_title)
            .setView(frame)
            .setPositiveButton(android.R.string.ok) { _, _ ->
                val label = input.text.toString().trim()
                background({ history.rename(e.id, label) }) { reload() }
            }
            .setNegativeButton(android.R.string.cancel, null)
            .show()
    }

    private fun confirmClear() {
        AlertDialog.Builder(this)
            .setMessage(R.string.clear_confirm)
            .setPositiveButton(R.string.clear_all) { _, _ ->
                selected.clear()
                background({ history.clear() }) { reload() }
            }
            .setNegativeButton(android.R.string.cancel, null)
            .show()
    }

    /** Compares the two checked runs, the earlier one as A. */
    private fun compareSelected() {
        val runs = entries.filter { it.id in selected }.sortedBy { startedMillis(it.started) }
        if (runs.size != 2) return
        startActivity(
            Intent(this, CompareActivity::class.java)
                .putExtra(CompareActivity.EXTRA_A, runs[0].id)
                .putExtra(CompareActivity.EXTRA_B, runs[1].id),
        )
    }

    /** Lets the user pick JSON reports to import. */
    private fun pickReports() {
        val intent = Intent(Intent.ACTION_OPEN_DOCUMENT)
            .addCategory(Intent.CATEGORY_OPENABLE)
            // files people save are not always labeled as JSON
            .setType("*/*")
            .putExtra(Intent.EXTRA_MIME_TYPES, arrayOf(ReportProvider.TYPE, "text/plain", "application/octet-stream"))
            .putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
        startActivityForResult(intent, IMPORT_REQUEST)
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        @Suppress("DEPRECATION")
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != IMPORT_REQUEST || resultCode != RESULT_OK || data == null) return
        val uris = data.clipData?.let { c -> (0 until c.itemCount).map { c.getItemAt(it).uri } } ?: listOfNotNull(data.data)
        background({ importReports(uris) }) { (added, errors) ->
            val messages = mutableListOf<String>()
            if (added > 0) messages.add(resources.getQuantityString(R.plurals.imported, added, added))
            messages.addAll(errors)
            if (messages.isNotEmpty()) {
                Toast.makeText(this, messages.joinToString("\n"), if (errors.isEmpty()) Toast.LENGTH_SHORT else Toast.LENGTH_LONG).show()
            }
            reload()
        }
    }

    /** Imports reports, the oldest first, returning how many were added and why others were not. */
    private fun importReports(uris: List<Uri>): Pair<Int, List<String>> {
        val errors = ArrayList<String>()
        val reports = ArrayList<Pair<String, ScanReport>>()
        for (uri in uris) {
            val name = displayName(uri)
            try {
                val json = contentResolver.openInputStream(uri)!!.use { it.readBytes().toString(Charsets.UTF_8) }
                Mobile.checkReport(json)
                val report = ScanReport.parse(json)
                if (history.contains(report) || reports.any { it.second.sameRun(report) }) {
                    errors.add(getString(R.string.import_error, name, getString(R.string.import_duplicate)))
                    continue
                }
                reports.add(json to report)
            } catch (e: Exception) {
                errors.add(getString(R.string.import_error, name, e.message ?: e.toString()))
            }
        }
        val now = System.currentTimeMillis()
        for ((i, r) in reports.sortedBy { startedMillis(it.second.started) }.withIndex()) {
            history.add(r.first, r.second, now + i)
        }
        return reports.size to errors
    }

    private fun displayName(uri: Uri): String = try {
        contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use {
            if (it.moveToFirst()) it.getString(0) else null
        }
    } catch (_: Exception) {
        null
    } ?: uri.lastPathSegment ?: uri.toString()

    private fun selectableBackground() = TypedValue().let {
        theme.resolveAttribute(android.R.attr.selectableItemBackground, it, true)
        getDrawable(it.resourceId)
    }

    private fun dp(value: Int): Int =
        TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, value.toFloat(), resources.displayMetrics).toInt()

    companion object {
        private const val IMPORT_REQUEST = 1
        private const val MENU_RENAME = 1
        private const val MENU_DELETE = 2
        private const val STATE_SELECTED = "selected"
    }
}
