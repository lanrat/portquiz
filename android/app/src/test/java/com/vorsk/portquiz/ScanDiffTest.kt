package com.vorsk.portquiz

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.io.File

class ScanDiffTest {
    // the Go library's diff of its shared test reports, which the app shows
    private val diff = ScanDiff.parse(File("../../scan/testdata/diff/want.json").readText())

    @Test
    fun parsesTheGoDiff() {
        assertEquals(listOf("tcp4", "tcp6", "udp4"), diff.kinds.map { it.kind })
        val tcp = diff.kinds[0]
        assertEquals("tcp", tcp.aKind)
        assertEquals("tcp4", tcp.bKind)
        assertEquals(5, tcp.count(Change.OPENED))
        assertEquals(10, tcp.count(Change.CLOSED))
        assertEquals(6, tcp.count(Change.REASON_CHANGED))
        assertEquals(18, tcp.count(Change.UNCHANGED))
        assertEquals(11, tcp.count(Change.ONLY_B))
        assertEquals(21, tcp.changed)
        assertEquals(39, tcp.compared)
        assertEquals(12.0, tcp.aMedian!!, 0.0)
        assertEquals(StatusChange(6, 10, "open", "dropped"), tcp.changes[0])
        assertEquals(ChangeRange(41, 50, Change.ONLY_B), tcp.ranges.last())
        val v6 = diff.kinds[1]
        assertEquals("", v6.aKind)
        assertNull(v6.aMedian)
        assertEquals(24, diff.changed)
    }

    @Test
    fun rangesCoverEveryPortOnce() {
        for (k in diff.kinds) {
            var prev = 0
            for (r in k.ranges) {
                assert(r.first > prev)
                prev = r.last
            }
            assertEquals(Change.entries.sumOf { k.count(it) }, k.ranges.sumOf { it.last - it.first + 1 })
        }
    }
}
