package io.github.lanrat.portquiz

import android.content.Context
import io.github.lanrat.portquiz.mobile.Mobile

/** The settings of the last scan, restored when the app opens. */
class Settings(context: Context) {
    private val prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE)

    val host: String get() = prefs.getString(HOST, "")!!
    val ports: String get() = prefs.getString(PORTS, "")!!
    val tcp: Boolean get() = prefs.getBoolean(TCP, true)
    val udp: Boolean get() = prefs.getBoolean(UDP, false)
    val password: String get() = prefs.getString(PASSWORD, null) ?: DEFAULT_PASSWORD

    fun save(host: String, ports: String, tcp: Boolean, udp: Boolean, password: String) {
        prefs.edit()
            .putString(HOST, host)
            .putString(PORTS, ports)
            .putBoolean(TCP, tcp)
            .putBoolean(UDP, udp)
            .putString(PASSWORD, password)
            .apply()
    }

    companion object {
        /** The default password, the same as the command line client's. */
        val DEFAULT_PASSWORD: String by lazy { Mobile.newConfig().password }

        private const val HOST = "host"
        private const val PORTS = "ports"
        private const val TCP = "tcp"
        private const val UDP = "udp"
        private const val PASSWORD = "password"
    }
}
