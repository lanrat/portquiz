package com.vorsk.portquiz

import android.Manifest
import android.annotation.SuppressLint
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.View
import android.view.ViewGroup
import android.view.WindowInsets
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.ListView
import android.widget.ProgressBar
import android.widget.RadioGroup
import android.widget.Spinner
import android.widget.TextView

/** The app's only screen: scan settings, a start/stop button and the results. */
class MainActivity : Activity() {
    private lateinit var settings: Settings
    private lateinit var host: EditText
    private lateinit var ports: EditText
    private lateinit var password: EditText
    private lateinit var tcp: CheckBox
    private lateinit var udp: CheckBox
    private lateinit var ipVersion: Spinner
    private lateinit var advancedToggle: TextView
    private lateinit var advanced: View
    private lateinit var timeout: EditText
    private lateinit var adaptive: CheckBox
    private lateinit var rate: EditText
    private lateinit var retries: EditText
    private lateinit var parallel: EditText
    private lateinit var reset: Button
    private lateinit var start: Button
    private lateinit var progress: ProgressBar
    private lateinit var status: TextView
    private lateinit var filter: RadioGroup
    private lateinit var results: ListView
    private lateinit var noResults: TextView

    // the result lines that pass the filter, and how far into ScanState.lines
    // of which scan they have been brought up to date
    private val shown = ArrayList<ScanState.Line>()
    private lateinit var adapter: ArrayAdapter<ScanState.Line>
    private var shownFilter: ResultsFilter? = null
    private var shownGeneration = -1
    private var shownCount = 0

    private val main = Handler(Looper.getMainLooper())

    // the theme's secondary text color, for closed ports and the status line
    private val secondaryText by lazy {
        val attrs = obtainStyledAttributes(intArrayOf(android.R.attr.textColorSecondary))
        try {
            attrs.getColorStateList(0)!!
        } finally {
            attrs.recycle()
        }
    }
    private val listener: () -> Unit = { render() }

    // refreshes the time remaining while a scan runs
    private val tick = object : Runnable {
        override fun run() {
            render()
            if (ScanState.running) main.postDelayed(this, 1000)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        drawBehindSystemBars()
        setContentView(R.layout.activity_main)
        settings = Settings(this)

        host = findViewById(R.id.host)
        ports = findViewById(R.id.ports)
        password = findViewById(R.id.password)
        tcp = findViewById(R.id.tcp)
        udp = findViewById(R.id.udp)
        ipVersion = findViewById(R.id.ip_version)
        advancedToggle = findViewById(R.id.advanced_toggle)
        advanced = findViewById(R.id.advanced)
        timeout = findViewById(R.id.timeout)
        adaptive = findViewById(R.id.adaptive)
        rate = findViewById(R.id.rate)
        retries = findViewById(R.id.retries)
        parallel = findViewById(R.id.parallel)
        reset = findViewById(R.id.reset)
        start = findViewById(R.id.start)
        progress = findViewById(R.id.progress)
        status = findViewById(R.id.status)
        filter = findViewById(R.id.filter)
        results = findViewById(R.id.results)
        noResults = findViewById(R.id.no_results)

        ipVersion.adapter = ArrayAdapter.createFromResource(
            this, R.array.ip_versions, android.R.layout.simple_spinner_item,
        ).apply { setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item) }

        adapter = object : ArrayAdapter<ScanState.Line>(this, android.R.layout.simple_list_item_1, shown) {
            override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
                val view = super.getView(position, convertView, parent) as TextView
                val line = getItem(position)!!
                view.text = line.text
                if (line.open) view.setTextColor(getColor(R.color.open)) else view.setTextColor(secondaryText)
                return view
            }
        }
        results.adapter = adapter

        applyInsets(findViewById(R.id.root))

        findViewById<TextView>(R.id.version).text = BuildConfig.VERSION_NAME
        findViewById<View>(R.id.github).setOnClickListener {
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(getString(R.string.github_url))))
        }
        advancedToggle.setOnClickListener { showAdvanced(advanced.visibility != View.VISIBLE) }
        reset.setOnClickListener { resetAdvanced() }
        start.setOnClickListener { if (ScanState.running) stopScan() else startScan() }

        filter.check(
            when (settings.resultsFilter) {
                ResultsFilter.ALL -> R.id.filter_all
                ResultsFilter.OPEN -> R.id.filter_open
                ResultsFilter.CLOSED -> R.id.filter_closed
            }
        )
        filter.setOnCheckedChangeListener { _, _ ->
            settings.resultsFilter = selectedFilter()
            render()
        }

        // views restore what was typed themselves when the activity is recreated
        if (savedInstanceState == null) showSettings(settings.load())
        showAdvanced(settings.advancedShown)
    }

    override fun onStart() {
        super.onStart()
        ScanState.addListener(listener)
        render()
        main.post(tick)
    }

    override fun onStop() {
        ScanState.removeListener(listener)
        main.removeCallbacks(tick)
        super.onStop()
    }

    /** Fills in the settings form. */
    private fun showSettings(s: ScanSettings) {
        host.setText(s.host)
        ports.setText(s.ports)
        password.setText(s.password)
        tcp.isChecked = s.tcp
        udp.isChecked = s.udp
        ipVersion.setSelection(s.ipVersion.ordinal)
        showAdvancedSettings(s.timeoutMillis, s.adaptive, s.rate, s.retries, s.parallel)
    }

    // the fields hold plain ASCII numbers, as parseNumber reads them back, so
    // they are deliberately not formatted for the locale
    @SuppressLint("SetTextI18n")
    private fun showAdvancedSettings(timeoutMillis: Long, adaptive: Boolean, rate: Long, retries: Long, parallel: Long) {
        timeout.setText(timeoutMillis.toString())
        this.adaptive.isChecked = adaptive
        this.rate.setText(rate.toString())
        // blank means the default number of retries
        this.retries.setText(if (retries < 0) "" else retries.toString())
        this.parallel.setText(parallel.toString())
    }

    private fun resetAdvanced() {
        val d = Settings.DEFAULTS
        showAdvancedSettings(d.timeoutMillis, d.adaptive, d.rate, d.retries, d.parallel)
    }

    /** Reads the settings form; blank or invalid fields get their defaults. */
    private fun readSettings(): ScanSettings {
        val d = Settings.DEFAULTS
        return ScanSettings(
            host = host.text.toString().trim(),
            ports = ports.text.toString().trim(),
            tcp = tcp.isChecked,
            udp = udp.isChecked,
            password = password.text.toString().ifEmpty { d.password },
            ipVersion = IpVersion.entries[ipVersion.selectedItemPosition],
            timeoutMillis = parseNumber(timeout.text, d.timeoutMillis),
            adaptive = adaptive.isChecked,
            rate = parseNumber(rate.text, d.rate),
            retries = parseNumber(retries.text, d.retries),
            parallel = parseNumber(parallel.text, d.parallel),
        )
    }

    private fun showAdvanced(show: Boolean) {
        advanced.visibility = if (show) View.VISIBLE else View.GONE
        advancedToggle.setText(if (show) R.string.advanced_shown else R.string.advanced_hidden)
        settings.advancedShown = show
    }

    private fun selectedFilter(): ResultsFilter = when (filter.checkedRadioButtonId) {
        R.id.filter_open -> ResultsFilter.OPEN
        R.id.filter_closed -> ResultsFilter.CLOSED
        else -> ResultsFilter.ALL
    }

    private fun startScan() {
        val s = readSettings()
        if (s.host.isEmpty()) {
            host.error = getString(R.string.error_no_host)
            return
        }
        if (!s.tcp && !s.udp) {
            udp.error = getString(R.string.error_no_protocol)
            return
        }
        udp.error = null

        // show the values the scan uses, with defaults filled in
        showSettings(s)
        // the scan service reads the saved settings
        settings.save(s)

        // the progress notification needs this permission on Android 13+;
        // scanning works without it
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 0)
        }

        startForegroundService(Intent(this, ScanService::class.java).setAction(ScanService.ACTION_START))
        main.removeCallbacks(tick)
        main.post(tick)
    }

    private fun stopScan() {
        startService(Intent(this, ScanService::class.java).setAction(ScanService.ACTION_STOP))
    }

    private fun render() {
        val running = ScanState.running
        start.setText(if (running) R.string.stop else R.string.start)
        for (v in listOf(host, ports, password, tcp, udp, ipVersion, timeout, adaptive, rate, retries, parallel, reset)) {
            v.isEnabled = !running
        }

        progress.visibility = if (ScanState.startedAt == 0L) View.INVISIBLE else View.VISIBLE
        progress.isIndeterminate = running && ScanState.total == 0L
        progress.max = ScanState.total.toInt().coerceAtLeast(1)
        progress.progress = ScanState.done.toInt()

        var text = ScanService.statusText(this)
        if (!running && ScanState.missed > 0) {
            text += "\n" + getString(R.string.status_missed, ScanState.missed)
        }
        status.text = text
        if (ScanState.error != null || (!running && ScanState.missed > 0)) {
            status.setTextColor(getColor(R.color.warning))
        } else {
            status.setTextColor(secondaryText)
        }
        renderResults()
    }

    /** Brings the shown results up to date with ScanState and the filter. */
    private fun renderResults() {
        val f = selectedFilter()
        val lines = ScanState.lines
        if (f != shownFilter || ScanState.generation != shownGeneration || lines.size < shownCount) {
            // a new filter or scan: start over
            shown.clear()
            shownCount = 0
            shownFilter = f
            shownGeneration = ScanState.generation
        }
        for (i in shownCount until lines.size) {
            if (f.matches(lines[i])) shown.add(lines[i])
        }
        shownCount = lines.size
        adapter.notifyDataSetChanged()

        val empty = shown.isEmpty() && lines.isNotEmpty()
        noResults.visibility = if (empty) View.VISIBLE else View.GONE
        results.visibility = if (empty) View.GONE else View.VISIBLE
    }

    /**
     * Draws behind the status and navigation bars on every Android version,
     * as Android 15 and later require, so [applyInsets] works the same way
     * everywhere.
     */
    private fun drawBehindSystemBars() {
        when {
            // Android 15 and later always draw behind the system bars
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM -> {}
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.R ->
                @Suppress("DEPRECATION")
                window.setDecorFitsSystemWindows(false)
            else ->
                @Suppress("DEPRECATION")
                window.decorView.systemUiVisibility = window.decorView.systemUiVisibility or
                    View.SYSTEM_UI_FLAG_LAYOUT_STABLE or
                    View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or
                    View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
        }
    }

    /** Keeps content clear of the status bar, navigation bar and keyboard. */
    private fun applyInsets(root: View) {
        val left = root.paddingLeft
        val top = root.paddingTop
        val right = root.paddingRight
        val bottom = root.paddingBottom
        root.setOnApplyWindowInsetsListener { v, insets ->
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                val bars = insets.getInsets(
                    WindowInsets.Type.systemBars() or WindowInsets.Type.displayCutout() or WindowInsets.Type.ime()
                )
                v.setPadding(left + bars.left, top + bars.top, right + bars.right, bottom + bars.bottom)
            } else {
                @Suppress("DEPRECATION")
                v.setPadding(
                    left + insets.systemWindowInsetLeft, top + insets.systemWindowInsetTop,
                    right + insets.systemWindowInsetRight, bottom + insets.systemWindowInsetBottom,
                )
            }
            insets
        }
    }
}
