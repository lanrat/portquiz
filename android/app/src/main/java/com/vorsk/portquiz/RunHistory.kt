package com.vorsk.portquiz

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

/** A kind's results in a [HistoryEntry]: how many ports were open of those tested. */
data class KindSummary(val kind: String, val open: Int, val tested: Int)

/** A run kept in the [RunHistory], with what the list of runs shows of it. */
data class HistoryEntry(
    val id: String,
    /** When the run was added, in milliseconds since the epoch. */
    val added: Long,
    val started: String,
    val finished: String,
    val host: String,
    val label: String,
    val client: String,
    val clientAddress: String,
    val tcpRequests: String,
    val canceled: Boolean,
    val kinds: List<KindSummary>,
) {
    fun toJson(): JSONObject = JSONObject()
        .put("id", id)
        .put("added", added)
        .put("started", started)
        .put("finished", finished)
        .put("host", host)
        .put("label", label)
        .put("client", client)
        .put("client_address", clientAddress)
        .put("tcp_requests", tcpRequests)
        .put("canceled", canceled)
        .put("kinds", JSONArray(kinds.map { JSONObject().put("kind", it.kind).put("open", it.open).put("tested", it.tested) }))

    companion object {
        fun of(id: String, added: Long, r: ScanReport) = HistoryEntry(
            id = id,
            added = added,
            started = r.started,
            finished = r.finished,
            host = r.host,
            label = r.label,
            client = r.client,
            clientAddress = r.clientAddress,
            tcpRequests = r.tcpRequests,
            canceled = r.canceled,
            kinds = r.kinds.map { KindSummary(it.kind, it.open, it.tested) },
        )

        fun parse(o: JSONObject): HistoryEntry {
            val kinds = o.optJSONArray("kinds") ?: JSONArray()
            return HistoryEntry(
                id = o.getString("id"),
                added = o.optLong("added"),
                started = o.optString("started"),
                finished = o.optString("finished"),
                host = o.optString("host"),
                label = o.optString("label"),
                client = o.optString("client"),
                clientAddress = o.optString("client_address"),
                tcpRequests = o.optString("tcp_requests"),
                canceled = o.optBoolean("canceled"),
                kinds = (0 until kinds.length()).map {
                    val k = kinds.getJSONObject(it)
                    KindSummary(k.optString("kind"), k.optInt("open"), k.optInt("tested"))
                },
            )
        }
    }
}

/**
 * The last [limit] runs, finished here or imported, newest first, for
 * comparing: each report is a file in dir, and an index lists them with what
 * the list of runs shows, so the list does not need every report read.
 * Everything stays on this phone. It is safe to use from any thread; reports
 * can be large, so use it off the main thread.
 */
class RunHistory(private val dir: File, private val limit: Int = LIMIT) {
    private val index = File(dir, "index.json")

    /** The runs kept, newest first. */
    fun list(): List<HistoryEntry> = synchronized(lock) { load() }

    /**
     * Adds a run's report, given as JSON and parsed, as the newest, and
     * removes the oldest runs beyond the limit. Returns the new entry.
     */
    fun add(json: String, report: ScanReport, added: Long = System.currentTimeMillis()): HistoryEntry = synchronized(lock) {
        val entries = load()
        var id = added.toString()
        var n = 1
        while (entries.any { it.id == id }) id = "$added-${n++}"
        dir.mkdirs()
        file(id).writeText(json)
        val entry = HistoryEntry.of(id, added, report)
        val kept = listOf(entry) + entries
        for (old in kept.drop(limit)) file(old.id).delete()
        store(kept.take(limit))
        entry
    }

    /** Whether a run of the same scan as report is kept. */
    fun contains(report: ScanReport): Boolean = list().any {
        it.started == report.started && it.finished == report.finished && it.host == report.host && it.client == report.client
    }

    /** The report of the run with this id, as JSON, or null if it is gone. */
    fun read(id: String): String? = synchronized(lock) {
        file(id).takeIf { it.isFile && load().any { e -> e.id == id } }?.readText()
    }

    /** Names a run, or removes its name if label is empty; the name is saved in its report too. */
    fun rename(id: String, label: String) = synchronized(lock) {
        val entries = load()
        val i = entries.indexOfFirst { it.id == id }
        if (i < 0) return@synchronized
        val f = file(id)
        val report = JSONObject(f.readText())
        if (label.isEmpty()) report.remove("label") else report.put("label", label)
        f.writeText(report.toString())
        store(entries.toMutableList().also { it[i] = it[i].copy(label = label) })
    }

    fun delete(id: String) = synchronized(lock) {
        val entries = load()
        file(id).delete()
        store(entries.filter { it.id != id })
    }

    fun clear() = synchronized(lock) {
        for (e in load()) file(e.id).delete()
        store(emptyList())
    }

    private fun file(id: String) = File(dir, "$id.json")

    private fun load(): List<HistoryEntry> {
        if (!index.isFile) return emptyList()
        return try {
            val a = JSONArray(index.readText())
            (0 until a.length()).map { HistoryEntry.parse(a.getJSONObject(it)) }
        } catch (_: Exception) {
            emptyList()
        }
    }

    /** Writes the index, replacing the old one only once the new one is complete. */
    private fun store(entries: List<HistoryEntry>) {
        dir.mkdirs()
        val tmp = File(dir, "index.json.tmp")
        tmp.writeText(JSONArray(entries.map { it.toJson() }).toString())
        if (!tmp.renameTo(index)) {
            index.delete()
            tmp.renameTo(index)
        }
        // remove reports that are no longer listed, as after a crash mid-change
        val ids = entries.map { "${it.id}.json" }.toSet()
        dir.listFiles()?.filter { it.name.endsWith(".json") && it.name != index.name && it.name !in ids }?.forEach { it.delete() }
    }

    companion object {
        const val LIMIT = 20
        private val lock = Any()

        fun of(context: Context) = RunHistory(File(context.filesDir, "history"))
    }
}
