package com.vorsk.portquiz

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.json.JSONObject

class RunHistoryTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun report(started: String, label: String = "") = """
        {"client": "android", "host": "example.com", "started": "$started", "finished": "$started",
         ${if (label.isEmpty()) "" else "\"label\": \"$label\","}
         "kinds": [{"kind": "tcp", "address": "192.0.2.1", "tested": 3, "open": 1,
                    "ranges": [{"first": 1, "last": 1, "status": "open"}, {"first": 2, "last": 3, "status": "dropped"}]}]}
    """.trimIndent()

    private fun add(h: RunHistory, started: String, added: Long, label: String = ""): HistoryEntry {
        val json = report(started, label)
        return h.add(json, ScanReport.parse(json), added)
    }

    @Test
    fun keepsTheNewestRuns() {
        val dir = tmp.newFolder()
        val h = RunHistory(dir, limit = 3)
        for (i in 1..5) add(h, "2026-10-1${i}T09:00:00Z", i.toLong())
        val list = h.list()
        assertEquals(listOf("5", "4", "3"), list.map { it.id })
        assertEquals("2026-10-15T09:00:00Z", list[0].started)
        assertEquals(listOf(KindSummary("tcp", 1, 3)), list[0].kinds)
        // the dropped runs' reports are gone too
        assertEquals(setOf("3.json", "4.json", "5.json", "index.json"), dir.list()!!.toSet())
        assertNull(h.read("1"))
        assertTrue(h.read("5")!!.contains("2026-10-15"))
        // another instance sees the same runs
        assertEquals(list, RunHistory(dir, limit = 3).list())
    }

    @Test
    fun idsAreUnique() {
        val h = RunHistory(tmp.newFolder())
        val a = add(h, "2026-10-11T09:00:00Z", 7)
        val b = add(h, "2026-10-12T09:00:00Z", 7)
        assertEquals("7", a.id)
        assertEquals("7-1", b.id)
    }

    @Test
    fun renamesDeletesAndClears() {
        val h = RunHistory(tmp.newFolder())
        add(h, "2026-10-11T09:00:00Z", 1, label = "old")
        add(h, "2026-10-12T09:00:00Z", 2)
        h.rename("1", "home wifi")
        assertEquals("home wifi", h.list().first { it.id == "1" }.label)
        assertEquals("home wifi", JSONObject(h.read("1")!!).getString("label"))
        h.rename("1", "")
        assertEquals("", h.list().first { it.id == "1" }.label)
        assertFalse(JSONObject(h.read("1")!!).has("label"))
        h.delete("2")
        assertEquals(listOf("1"), h.list().map { it.id })
        h.clear()
        assertTrue(h.list().isEmpty())
    }

    @Test
    fun knowsRunsAlreadyKept() {
        val h = RunHistory(tmp.newFolder())
        add(h, "2026-10-11T09:00:00Z", 1)
        assertTrue(h.contains(ScanReport.parse(report("2026-10-11T09:00:00Z", label = "renamed"))))
        assertFalse(h.contains(ScanReport.parse(report("2026-10-12T09:00:00Z"))))
    }

    @Test
    fun aDamagedIndexIsEmpty() {
        val dir = tmp.newFolder()
        java.io.File(dir, "index.json").writeText("{not json")
        assertTrue(RunHistory(dir).list().isEmpty())
    }
}
