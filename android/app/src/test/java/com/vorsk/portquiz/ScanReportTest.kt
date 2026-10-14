package com.vorsk.portquiz

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class ScanReportTest {
    /** A report as the Go library writes it, for a dual stack TCP scan of ports 1 to 30. */
    private val json = """
        {
          "version": "v1.6.0",
          "host": "example.com",
          "started": "2026-10-13T09:00:00Z",
          "finished": "2026-10-13T09:00:10Z",
          "duration_seconds": 10.5,
          "tested_per_second": 5.7,
          "canceled": true,
          "options": {"ports": "1-30", "tcp": true, "udp": false, "ipv4": false, "ipv6": false,
                      "timeout_ms": 2000, "adaptive": true, "retries": -1, "parallel": 500, "rate": 500, "multi": 1},
          "kinds": [
            {"kind": "tcp4", "address": "192.0.2.1", "tested": 30, "untested": 0,
             "open": 3, "rejected": 2, "dropped": 20, "interfered": 4, "errors": 1, "late": 1, "retries": 1,
             "timeout_ms": 250.5, "open_rtt_ms": {"min": 1.5, "median": 2, "p95": 3.25, "max": 4},
             "ranges": [
               {"first": 1, "last": 10, "status": "dropped"},
               {"first": 11, "last": 12, "status": "rejected"},
               {"first": 13, "last": 22, "status": "dropped"},
               {"first": 23, "last": 25, "status": "open"},
               {"first": 26, "last": 29, "status": "interfered"},
               {"first": 30, "last": 30, "status": "error"}
             ]},
            {"kind": "tcp6", "address": "2001:db8::1", "tested": 27, "untested": 3,
             "open": 4, "rejected": 0, "dropped": 23, "interfered": 0, "errors": 0, "late": 0, "retries": 1,
             "timeout_ms": 2000,
             "ranges": [
               {"first": 1, "last": 21, "status": "dropped"},
               {"first": 22, "last": 23, "status": "open"},
               {"first": 24, "last": 25, "status": "dropped"},
               {"first": 26, "last": 27, "status": "open"}
             ]}
          ]
        }
    """.trimIndent()

    private val report = ScanReport.parse(json)
    private val v4 = report.kinds[0]
    private val v6 = report.kinds[1]

    @Test
    fun parsesTheReport() {
        assertEquals("example.com", report.host)
        assertEquals(10.5, report.seconds, 0.0)
        assertEquals(5.7, report.perSecond, 0.0)
        assertTrue(report.canceled)
        assertEquals(2000L, report.timeoutMillis)
        assertTrue(report.adaptive)
        assertEquals(57, report.tested)
        assertEquals(3, report.untested)
        assertEquals(json, report.json)

        assertEquals("tcp4", v4.kind)
        assertEquals("tcp", v4.protocol)
        assertEquals("4", v4.ipVersion)
        assertEquals("192.0.2.1", v4.address)
        assertEquals(27, v4.closed)
        assertEquals(20, v4.count(PortStatus.DROPPED))
        assertEquals(1, v4.count(PortStatus.ERROR))
        assertEquals(1, v4.late)
        assertEquals(250.5, v4.timeoutMillis, 0.0)
        assertEquals(ResponseTimes(1.5, 2.0, 3.25, 4.0), v4.responseTimes)
        assertNull(v6.responseTimes)
        assertEquals(PortRange(26, 29, PortStatus.INTERFERED), v4.ranges[4])
    }

    @Test
    fun parsesUnknownStatusesAsErrors() {
        assertEquals(PortStatus.ERROR, PortStatus.of("something new"))
        assertEquals(PortStatus.OPEN, PortStatus.of("open"))
    }

    @Test
    fun findsThePortStatus() {
        assertEquals(PortStatus.DROPPED, v4.statusOf(1))
        assertEquals(PortStatus.REJECTED, v4.statusOf(12))
        assertEquals(PortStatus.OPEN, v4.statusOf(24))
        assertEquals(PortStatus.ERROR, v4.statusOf(30))
        assertNull(v4.statusOf(31))
        assertNull(v6.statusOf(28))
    }

    @Test
    fun mergesClosedRangesIntoLargestRuns() {
        val runs = v4.largestClosed(5)
        // 1-22 is dropped, rejected and dropped again, so it is mixed
        assertEquals(PortRange(1, 22, PortStatus.DROPPED) to null, runs[0])
        assertEquals(PortRange(26, 30, PortStatus.INTERFERED) to null, runs[1])
        assertEquals(2, runs.size)
        assertEquals(1, v4.largestClosed(1).size)
        val single = v6.largestClosed(5)
        assertEquals(listOf(21, 2), single.map { it.first.size })
        assertEquals(PortStatus.DROPPED, single[0].second)
    }

    @Test
    fun findsWhereIpVersionsDiffer() {
        val diffs = report.versionDifferences()
        assertEquals(setOf("tcp"), diffs.keys)
        assertEquals(
            listOf(
                VersionDifference("tcp", PortRange(22, 22, PortStatus.OPEN), openOverIpv6 = true),
                VersionDifference("tcp", PortRange(24, 25, PortStatus.DROPPED), openOverIpv6 = false),
                VersionDifference("tcp", PortRange(26, 27, PortStatus.OPEN), openOverIpv6 = true),
            ),
            diffs["tcp"],
        )
    }

    @Test
    fun matchingVersionsHaveNoDifferences() {
        val same = ScanReport.parse(
            """
            {"host": "h", "kinds": [
              {"kind": "udp4", "ranges": [{"first": 53, "last": 53, "status": "open"}]},
              {"kind": "udp6", "ranges": [{"first": 53, "last": 53, "status": "open"}]}
            ]}
            """.trimIndent(),
        )
        assertEquals(mapOf("udp" to emptyList<VersionDifference>()), same.versionDifferences())
        assertFalse(same.canceled)
    }

    @Test
    fun singleVersionScansHaveNoComparison() {
        val single = ScanReport.parse("""{"host": "h", "kinds": [{"kind": "tcp", "ranges": []}]}""")
        assertTrue(single.versionDifferences().isEmpty())
        assertEquals("", single.kinds[0].ipVersion)
    }

    @Test
    fun servicesAreUniqueAndValid() {
        val keys = ScanReport.SERVICES.map { it.port to it.protocol }
        assertEquals(keys.size, keys.toSet().size)
        assertTrue(ScanReport.SERVICES.all { it.port in 1..65535 && it.protocol in setOf("tcp", "udp") })
    }

    @Test
    fun readsTheBrowserPageReports() {
        val r = ScanReport.parse(File("../../scan/testdata/diff/a.json").readText())
        assertEquals("browser", r.client)
        assertEquals("home wifi", r.label)
        assertEquals("198.51.100.7", r.clientAddress)
        assertEquals("http", r.tcpRequests)
        assertEquals("2026-10-13T09:30:00.000Z", r.started)
        val tcp = r.kinds[0]
        // the port the browser refused has no result
        assertEquals(39, tcp.tested)
        assertNull(tcp.statusOf(21))
        assertEquals(PortStatus.DROPPED, tcp.statusOf(22))
        assertTrue(r.sameRun(ScanReport.parse(File("../../scan/testdata/diff/a.json").readText())))
        assertFalse(r.sameRun(ScanReport.parse(File("../../scan/testdata/diff/b.json").readText())))
    }
}
