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
    fun roundsAndFormatsEstimates() {
        assertEquals(0L, roundEta(-1_000))
        assertEquals(5_000L, roundEta(1))
        assertEquals(45_000L, roundEta(41_000))
        assertEquals(60_000L, roundEta(59_000))
        assertEquals(80_000L, roundEta(84_000))
        assertEquals(90_000L, roundEta(86_000))
        assertEquals(780_000L, roundEta(760_000))
        assertEquals("45s", formatEta(45_000))
        assertEquals("1m", formatEta(60_000))
        assertEquals("3m 20s", formatEta(200_000))
        assertEquals("12m", formatEta(720_000))
        assertEquals("1h 5m", formatEta(3_900_000))
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

            ScanState.progress(22, 100, 1, -1)
            assertNull(ScanState.remaining())
            ScanState.progress(22, 100, 1, 60_000)
            assertTrue(ScanState.remaining()!! in 59_000L..60_000L)
            assertEquals(22L, ScanState.done)
            assertEquals(100L, ScanState.total)
            assertEquals(1L, ScanState.open)

            ScanState.finish(canceled = true, missed = 2, error = null)
            assertFalse(ScanState.running)
            assertTrue(ScanState.canceled)
            assertEquals(2L, ScanState.missed)
            assertNull(ScanState.remaining())
            assertEquals(6, notified)
        } finally {
            ScanState.removeListener(listener)
        }
    }

    @Test
    fun namesTheServerWithItsAddresses() {
        ScanState.begin("example.com")
        assertEquals("example.com", ScanState.server())
        ScanState.resolved("192.0.2.1")
        assertEquals("example.com (192.0.2.1)", ScanState.server())
        ScanState.resolved("192.0.2.1, 2001:db8::1")
        assertEquals("example.com (192.0.2.1, 2001:db8::1)", ScanState.server())
        ScanState.begin("[2001:db8::1]")
        ScanState.resolved("2001:db8::1")
        assertEquals("[2001:db8::1]", ScanState.server())
        ScanState.finish(canceled = false, missed = 0, error = null)
    }

    @Test
    fun beginClearsThePreviousScan() {
        ScanState.begin("a")
        ScanState.addRange("udp", 53, 53, true)
        ScanState.finish(canceled = false, missed = 0, error = "boom")
        ScanState.resolved("192.0.2.1")
        ScanState.begin("b")
        assertEquals("b", ScanState.host)
        assertEquals("", ScanState.addresses)
        assertTrue(ScanState.lines.isEmpty())
        assertNull(ScanState.error)
    }
}
