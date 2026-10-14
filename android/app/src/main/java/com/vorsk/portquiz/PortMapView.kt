package com.vorsk.portquiz

import android.annotation.SuppressLint
import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import kotlin.math.max
import kotlin.math.min

/** A run of ports with the same value, the index of one of a map's colors. */
data class MapRange(val first: Int, val last: Int, val value: Int)

/**
 * A strip showing one kind's results across a span of ports: each column
 * covers a run of ports and is split from the bottom up in proportion to how
 * many of them had each status, so blocked ranges stand out. Any status
 * present in a column gets at least a sliver, so lone open ports stay
 * visible. Touching it reports the ports under the finger to [onInspect].
 * It can also show other values than statuses, such as how ports changed
 * between two runs.
 */
class PortMapView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
) : View(context, attrs) {

    /** Counts per column of ports with each value. */
    private var columns: Array<IntArray> = emptyArray()
    private var ranges: List<MapRange>? = null
    private var paints: List<Paint> = emptyList()
    private var firstPort = 1
    private var lastPort = 1

    /** Receives the ports under a touch, with the count of each value. */
    var onInspect: ((first: Int, last: Int, counts: IntArray) -> Unit)? = null

    private val track = Paint().apply { color = context.getColor(R.color.map_track) }
    private val columnWidth = resources.displayMetrics.density * 2
    private val minSliver = resources.displayMetrics.density * 3

    /** Shows kind's results from port first to last, with counts in [PortStatus] order. */
    fun show(kind: KindReport, first: Int, last: Int) = show(
        kind.ranges.map { MapRange(it.first, it.last, it.status.ordinal) },
        PortStatus.entries.map { statusColor(it) },
        first, last,
    )

    /**
     * Shows ranges from port first to last, each value drawn in the color
     * with that index in colors, from the bottom up.
     */
    fun show(ranges: List<MapRange>, colors: List<Int>, first: Int, last: Int) {
        this.ranges = ranges
        paints = colors.map { Paint().apply { color = context.getColor(it) } }
        firstPort = first
        lastPort = last
        columns = emptyArray()
        requestLayout()
        invalidate()
    }

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        val height = (resources.displayMetrics.density * 64).toInt()
        setMeasuredDimension(
            getDefaultSize(suggestedMinimumWidth, widthMeasureSpec),
            resolveSize(height, heightMeasureSpec),
        )
    }

    override fun onSizeChanged(w: Int, h: Int, oldw: Int, oldh: Int) {
        columns = emptyArray()
    }

    /** The first port of column i; column i covers up to the next column's first port. */
    private fun columnStart(i: Int, count: Int): Int =
        firstPort + ((lastPort - firstPort + 1).toLong() * i / count).toInt()

    private fun buildColumns() {
        val rs = ranges ?: return
        val count = max(1, min((width / columnWidth).toInt(), lastPort - firstPort + 1))
        val cols = Array(count) { IntArray(paints.size) }
        var col = 0
        for (r in rs) {
            var port = max(r.first, firstPort)
            val end = min(r.last, lastPort)
            while (port <= end) {
                while (col + 1 < count && columnStart(col + 1, count) <= port) col++
                val colEnd = if (col + 1 < count) columnStart(col + 1, count) - 1 else lastPort
                val upTo = min(end, colEnd)
                cols[col][r.value] += upTo - port + 1
                port = upTo + 1
            }
        }
        columns = cols
    }

    override fun onDraw(canvas: Canvas) {
        if (ranges == null || width == 0) return
        if (columns.isEmpty()) buildColumns()
        val h = height.toFloat()
        canvas.drawRect(0f, 0f, width.toFloat(), h, track)
        val colW = width.toFloat() / columns.size
        for ((i, counts) in columns.withIndex()) {
            val size = columnStart(i + 1, columns.size) - columnStart(i, columns.size)
            if (size <= 0) continue
            val heights = segmentHeights(counts, size, h, minSliver)
            var bottom = h
            for ((value, segment) in heights.withIndex()) {
                if (segment <= 0f) continue
                val top = bottom - segment
                canvas.drawRect(i * colW, top, (i + 1) * colW, bottom, paints[value])
                bottom = top
            }
        }
    }

    // the map is a read-only display: touching it only inspects, so there
    // is nothing for performClick to do
    @SuppressLint("ClickableViewAccessibility")
    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (columns.isEmpty()) return false
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE -> {
                parent?.requestDisallowInterceptTouchEvent(true)
                val i = (event.x / width * columns.size).toInt().coerceIn(0, columns.size - 1)
                val last = if (i + 1 < columns.size) columnStart(i + 1, columns.size) - 1 else lastPort
                onInspect?.invoke(columnStart(i, columns.size), last, columns[i])
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                parent?.requestDisallowInterceptTouchEvent(false)
            }
        }
        return true
    }

    companion object {
        /** The color for a status on the port map and its legend. */
        fun statusColor(status: PortStatus): Int = when (status) {
            PortStatus.OPEN -> R.color.map_open
            PortStatus.REJECTED -> R.color.map_rejected
            PortStatus.DROPPED -> R.color.map_dropped
            PortStatus.INTERFERED -> R.color.map_interfered
            PortStatus.ERROR -> R.color.map_error
        }

        /** The color for a change on a comparison's map and its legend. */
        fun changeColor(change: Change): Int = when (change) {
            Change.OPENED -> R.color.map_open
            Change.CLOSED -> R.color.map_rejected
            Change.REASON_CHANGED -> R.color.map_dropped
            Change.UNCHANGED -> R.color.map_unchanged
            Change.ONLY_A, Change.ONLY_B -> R.color.map_not_compared
        }
    }
}

/**
 * The height of each status's segment in a map column of the given height
 * whose size ports had counts of each status. Segments are in proportion,
 * except that every status present gets at least minimum, so a few closed
 * ports among many open ones, or the reverse, stay visible; the extra height
 * comes from the tallest segment, so the column still fits.
 */
fun segmentHeights(counts: IntArray, size: Int, height: Float, minimum: Float): FloatArray {
    val heights = FloatArray(counts.size)
    if (size <= 0) return heights
    for (i in counts.indices) {
        if (counts[i] > 0) heights[i] = max(height * counts[i] / size, minimum)
    }
    val excess = heights.sum() - height
    if (excess > 0f) {
        val tallest = heights.indices.maxBy { heights[it] }
        heights[tallest] = max(heights[tallest] - excess, minimum)
    }
    return heights
}
