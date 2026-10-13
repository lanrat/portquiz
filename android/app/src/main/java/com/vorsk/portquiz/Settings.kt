package com.vorsk.portquiz

import android.content.Context
import com.vorsk.portquiz.mobile.Config
import com.vorsk.portquiz.mobile.Mobile

/** Stores the settings of the last scan, restored when the app opens. */
class Settings(context: Context) {
    private val prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE)

    /** The settings of the last scan, or the defaults. */
    fun load(): ScanSettings = ScanSettings(
        host = prefs.getString(HOST, "")!!,
        ports = prefs.getString(PORTS, "")!!,
        tcp = prefs.getBoolean(TCP, true),
        udp = prefs.getBoolean(UDP, false),
        password = prefs.getString(PASSWORD, null) ?: DEFAULTS.password,
        ipVersion = enumOr(prefs.getString(IP_VERSION, null), IpVersion.AUTO),
        timeoutMillis = prefs.getLong(TIMEOUT, DEFAULTS.timeoutMillis),
        adaptive = prefs.getBoolean(ADAPTIVE, DEFAULTS.adaptive),
        rate = prefs.getLong(RATE, DEFAULTS.rate),
        retries = prefs.getLong(RETRIES, DEFAULTS.retries),
        parallel = prefs.getLong(PARALLEL, DEFAULTS.parallel),
    )

    fun save(s: ScanSettings) {
        prefs.edit()
            .putString(HOST, s.host)
            .putString(PORTS, s.ports)
            .putBoolean(TCP, s.tcp)
            .putBoolean(UDP, s.udp)
            .putString(PASSWORD, s.password)
            .putString(IP_VERSION, s.ipVersion.name)
            .putLong(TIMEOUT, s.timeoutMillis)
            .putBoolean(ADAPTIVE, s.adaptive)
            .putLong(RATE, s.rate)
            .putLong(RETRIES, s.retries)
            .putLong(PARALLEL, s.parallel)
            .apply()
    }

    var resultsFilter: ResultsFilter
        get() = enumOr(prefs.getString(RESULTS_FILTER, null), ResultsFilter.ALL)
        set(value) = prefs.edit().putString(RESULTS_FILTER, value.name).apply()

    var advancedShown: Boolean
        get() = prefs.getBoolean(ADVANCED_SHOWN, false)
        set(value) = prefs.edit().putBoolean(ADVANCED_SHOWN, value).apply()

    companion object {
        /**
         * The scan defaults, the same as the command line client's: they come
         * from the Go library so the two cannot drift apart.
         */
        val DEFAULTS: Config by lazy { Mobile.newConfig() }

        private inline fun <reified T : Enum<T>> enumOr(name: String?, default: T): T =
            enumValues<T>().firstOrNull { it.name == name } ?: default

        private const val HOST = "host"
        private const val PORTS = "ports"
        private const val TCP = "tcp"
        private const val UDP = "udp"
        private const val PASSWORD = "password"
        private const val IP_VERSION = "ip_version"
        private const val TIMEOUT = "timeout_millis"
        private const val ADAPTIVE = "adaptive"
        private const val RATE = "rate"
        private const val RETRIES = "retries"
        private const val PARALLEL = "parallel"
        private const val RESULTS_FILTER = "results_filter"
        private const val ADVANCED_SHOWN = "advanced_shown"
    }
}

/** The Go scan configuration for these settings. */
fun ScanSettings.toConfig(): Config = Mobile.newConfig().also {
    it.host = host
    it.ports = ports
    it.tcp = tcp
    it.udp = udp
    it.password = password
    it.iPv4 = ipVersion.ipv4
    it.iPv6 = ipVersion.ipv6
    it.timeoutMillis = timeoutMillis
    it.adaptive = adaptive
    it.rate = rate
    it.retries = retries
    it.parallel = parallel
    it.version = BuildConfig.VERSION_NAME
}
