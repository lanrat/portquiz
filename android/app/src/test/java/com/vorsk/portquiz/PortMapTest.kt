package com.vorsk.portquiz

import org.junit.Assert.assertEquals
import org.junit.Test

class PortMapTest {
    private fun counts(open: Int = 0, rejected: Int = 0, dropped: Int = 0, interfered: Int = 0, errors: Int = 0) =
        intArrayOf(open, rejected, dropped, interfered, errors)

    @Test
    fun segmentsAreInProportion() {
        val h = segmentHeights(counts(open = 50, dropped = 50), 100, 64f, 3f)
        assertEquals(32f, h[PortStatus.OPEN.ordinal], 0.001f)
        assertEquals(32f, h[PortStatus.DROPPED.ordinal], 0.001f)
        assertEquals(0f, h[PortStatus.REJECTED.ordinal], 0f)
    }

    @Test
    fun aFewClosedPortsAmongOpenOnesStayVisible() {
        // the case that hid closed ports: one dropped port among 363 open ones
        val h = segmentHeights(counts(open = 363, dropped = 1), 364, 64f, 3f)
        assertEquals(3f, h[PortStatus.DROPPED.ordinal], 0.001f)
        assertEquals(61f, h[PortStatus.OPEN.ordinal], 0.001f)
        assertEquals(64f, h.sum(), 0.001f)
    }

    @Test
    fun aFewOpenPortsAmongClosedOnesStayVisible() {
        val h = segmentHeights(counts(open = 1, rejected = 1, dropped = 362), 364, 64f, 3f)
        assertEquals(3f, h[PortStatus.OPEN.ordinal], 0.001f)
        assertEquals(3f, h[PortStatus.REJECTED.ordinal], 0.001f)
        assertEquals(58f, h[PortStatus.DROPPED.ordinal], 0.001f)
        assertEquals(64f, h.sum(), 0.001f)
    }

    @Test
    fun untestedPortsLeaveTheRestEmpty() {
        val h = segmentHeights(counts(open = 10), 100, 64f, 3f)
        assertEquals(6.4f, h.sum(), 0.001f)
        assertEquals(0f, segmentHeights(counts(), 0, 64f, 3f).sum(), 0f)
    }
}
