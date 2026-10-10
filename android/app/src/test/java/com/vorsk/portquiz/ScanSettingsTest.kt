package com.vorsk.portquiz

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ScanSettingsTest {
    @Test
    fun parsesNumbersWithDefaults() {
        assertEquals(2000L, parseNumber("2000", 5))
        assertEquals(30L, parseNumber(" 30 ", 5))
        assertEquals(0L, parseNumber("0", 5))
        assertEquals(5L, parseNumber("", 5))
        assertEquals(5L, parseNumber("  ", 5))
        assertEquals(5L, parseNumber("abc", 5))
        assertEquals(5L, parseNumber("-3", 5))
        assertEquals(-1L, parseNumber("", -1))
    }

    @Test
    fun filtersResults() {
        val open = ScanState.Line("OPEN tcp 22", true)
        val closed = ScanState.Line("CLOSED tcp 1-21", false)
        assertTrue(ResultsFilter.ALL.matches(open))
        assertTrue(ResultsFilter.ALL.matches(closed))
        assertTrue(ResultsFilter.OPEN.matches(open))
        assertFalse(ResultsFilter.OPEN.matches(closed))
        assertFalse(ResultsFilter.CLOSED.matches(open))
        assertTrue(ResultsFilter.CLOSED.matches(closed))
    }

    @Test
    fun mapsIpVersionsToFlags() {
        assertEquals(listOf(false to false, true to false, false to true, true to true),
            IpVersion.entries.map { it.ipv4 to it.ipv6 })
        // the dropdown in the layout lists them in this order
        assertEquals(listOf("AUTO", "IPV4", "IPV6", "BOTH"), IpVersion.entries.map { it.name })
    }

    @Test
    fun newScanIncreasesGeneration() {
        val before = ScanState.generation
        ScanState.begin("example.com")
        assertEquals(before + 1, ScanState.generation)
    }
}
