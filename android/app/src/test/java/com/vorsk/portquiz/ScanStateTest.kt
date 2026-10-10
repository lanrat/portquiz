package com.vorsk.portquiz

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ScanStateTest {
    @Test
    fun formatsDurations() {
        assertEquals("0s", formatDuration(0))
        assertEquals("45s", formatDuration(45_000))
        assertEquals("1m 0s", formatDuration(59_600))
        assertEquals("3m 20s", formatDuration(200_000))
        assertEquals("1h 5m", formatDuration(3_900_000))
    }

    @Test
    fun recordsRangesAndProgress() {
        var notified = 0
        val listener: () -> Unit = { notified++ }
        ScanState.addListener(listener)
        try {
            ScanState.begin("example.com")
            assertTrue(ScanState.running)
            assertTrue(ScanState.lines.isEmpty())

            ScanState.addRange("tcp", 1, 21, false)
            ScanState.addRange("tcp", 22, 22, true)
            assertEquals(
                listOf(ScanState.Line("CLOSED tcp 1-21", false), ScanState.Line("OPEN tcp 22", true)),
                ScanState.lines,
            )

            ScanState.progress(22, 100, 1)
            assertEquals(22L, ScanState.done)
            assertEquals(100L, ScanState.total)
            assertEquals(1L, ScanState.open)

            ScanState.finish(canceled = true, missed = 2, error = null)
            assertFalse(ScanState.running)
            assertTrue(ScanState.canceled)
            assertEquals(2L, ScanState.missed)
            assertNull(ScanState.remaining())
            assertEquals(5, notified)
        } finally {
            ScanState.removeListener(listener)
        }
    }

    @Test
    fun beginClearsThePreviousScan() {
        ScanState.begin("a")
        ScanState.addRange("udp", 53, 53, true)
        ScanState.finish(canceled = false, missed = 0, error = "boom")
        ScanState.begin("b")
        assertEquals("b", ScanState.host)
        assertTrue(ScanState.lines.isEmpty())
        assertNull(ScanState.error)
    }
}
