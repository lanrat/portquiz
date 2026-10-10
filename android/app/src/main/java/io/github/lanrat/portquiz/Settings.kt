package io.github.lanrat.portquiz

import android.content.Context

/** The settings of the last scan, restored when the app opens. */
class Settings(context: Context) {
    private val prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE)

    val host: String get() = prefs.getString(HOST, "")!!
    val ports: String get() = prefs.getString(PORTS, "")!!
    val tcp: Boolean get() = prefs.getBoolean(TCP, true)
    val udp: Boolean get() = prefs.getBoolean(UDP, false)

    fun save(host: String, ports: String, tcp: Boolean, udp: Boolean) {
        prefs.edit()
            .putString(HOST, host)
            .putString(PORTS, ports)
            .putBoolean(TCP, tcp)
            .putBoolean(UDP, udp)
            .apply()
    }

    private companion object {
        const val HOST = "host"
        const val PORTS = "ports"
        const val TCP = "tcp"
        const val UDP = "udp"
    }
}
