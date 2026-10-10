package com.vorsk.portquiz

import android.Manifest
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
import android.widget.TextView

/** The app's only screen: scan settings, a start/stop button and the results. */
class MainActivity : Activity() {
    private lateinit var host: EditText
    private lateinit var ports: EditText
    private lateinit var password: EditText
    private lateinit var tcp: CheckBox
    private lateinit var udp: CheckBox
    private lateinit var start: Button
    private lateinit var progress: ProgressBar
    private lateinit var status: TextView
    private lateinit var adapter: ArrayAdapter<ScanState.Line>

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

        host = findViewById(R.id.host)
        ports = findViewById(R.id.ports)
        password = findViewById(R.id.password)
        tcp = findViewById(R.id.tcp)
        udp = findViewById(R.id.udp)
        start = findViewById(R.id.start)
        progress = findViewById(R.id.progress)
        status = findViewById(R.id.status)

        adapter = object : ArrayAdapter<ScanState.Line>(this, android.R.layout.simple_list_item_1, ScanState.lines) {
            override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
                val view = super.getView(position, convertView, parent) as TextView
                val line = getItem(position)!!
                view.text = line.text
                if (line.open) view.setTextColor(getColor(R.color.open)) else view.setTextColor(secondaryText)
                return view
            }
        }
        findViewById<ListView>(R.id.results).adapter = adapter

        applyInsets(findViewById(R.id.root))

        findViewById<TextView>(R.id.version).text = BuildConfig.VERSION_NAME
        findViewById<View>(R.id.github).setOnClickListener {
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(getString(R.string.github_url))))
        }
        start.setOnClickListener { if (ScanState.running) stopScan() else startScan() }

        val settings = Settings(this)
        if (savedInstanceState == null) {
            host.setText(settings.host)
            ports.setText(settings.ports)
            password.setText(settings.password)
            tcp.isChecked = settings.tcp
            udp.isChecked = settings.udp
        }
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

    private fun startScan() {
        val server = host.text.toString().trim()
        if (server.isEmpty()) {
            host.error = getString(R.string.error_no_host)
            return
        }
        if (!tcp.isChecked && !udp.isChecked) {
            udp.error = getString(R.string.error_no_protocol)
            return
        }
        udp.error = null

        // an empty password means the default
        if (password.text.isEmpty()) password.setText(Settings.DEFAULT_PASSWORD)
        val pass = password.text.toString()

        Settings(this).save(server, ports.text.toString().trim(), tcp.isChecked, udp.isChecked, pass)

        // the progress notification needs this permission on Android 13+;
        // scanning works without it
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 0)
        }

        startForegroundService(
            Intent(this, ScanService::class.java)
                .setAction(ScanService.ACTION_START)
                .putExtra(ScanService.EXTRA_HOST, server)
                .putExtra(ScanService.EXTRA_PORTS, ports.text.toString().trim())
                .putExtra(ScanService.EXTRA_TCP, tcp.isChecked)
                .putExtra(ScanService.EXTRA_UDP, udp.isChecked)
                .putExtra(ScanService.EXTRA_PASSWORD, pass)
        )
        main.removeCallbacks(tick)
        main.post(tick)
    }

    private fun stopScan() {
        startService(Intent(this, ScanService::class.java).setAction(ScanService.ACTION_STOP))
    }

    private fun render() {
        val running = ScanState.running
        start.setText(if (running) R.string.stop else R.string.start)
        for (v in listOf(host, ports, password, tcp, udp)) v.isEnabled = !running

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
        adapter.notifyDataSetChanged()
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
