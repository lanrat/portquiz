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

/**
 * A strip showing one kind's results across a span of ports: each column
 * covers a run of ports and is split from the bottom up in proportion to how
 * many of them had each status, so blocked ranges stand out. Any status
 * present in a column gets at least a sliver, so lone open ports stay
 * visible. Touching it reports the ports under the finger to [onInspect].
 */
class PortMapView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
) : View(context, attrs) {

    /** Counts per column of ports with each status, in [PortStatus] order. */
    private var columns: Array<IntArray> = emptyArray()
    private var kind: KindReport? = null
    private var firstPort = 1
    private var lastPort = 1

    /** Receives the ports under a touch, and null when the touch ends. */
    var onInspect: ((first: Int, last: Int, counts: IntArray) -> Unit)? = null

    private val paints = PortStatus.entries.map { status ->
        Paint().apply { color = context.getColor(statusColor(status)) }
    }
    private val track = Paint().apply { color = context.getColor(R.color.map_track) }
    private val columnWidth = resources.displayMetrics.density * 2
    private val minSliver = resources.displayMetrics.density * 2

    /** Shows kind's results from port first to last. */
    fun show(kind: KindReport, first: Int, last: Int) {
        this.kind = kind
        firstPort = first
        lastPort = last
        columns = emptyArray()
        requestLayout()
        invalidate()
    }

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        val height = (resources.displayMetrics.density * 32).toInt()
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
        val k = kind ?: return
        val count = max(1, min((width / columnWidth).toInt(), lastPort - firstPort + 1))
        val cols = Array(count) { IntArray(PortStatus.entries.size) }
        var col = 0
        for (r in k.ranges) {
            var port = max(r.first, firstPort)
            val end = min(r.last, lastPort)
            while (port <= end) {
                while (col + 1 < count && columnStart(col + 1, count) <= port) col++
                val colEnd = if (col + 1 < count) columnStart(col + 1, count) - 1 else lastPort
                val upTo = min(end, colEnd)
                cols[col][r.status.ordinal] += upTo - port + 1
                port = upTo + 1
            }
        }
        columns = cols
    }

    override fun onDraw(canvas: Canvas) {
        if (kind == null || width == 0) return
        if (columns.isEmpty()) buildColumns()
        val h = height.toFloat()
        canvas.drawRect(0f, 0f, width.toFloat(), h, track)
        val colW = width.toFloat() / columns.size
        for ((i, counts) in columns.withIndex()) {
            val size = columnStart(i + 1, columns.size) - columnStart(i, columns.size)
            if (size <= 0) continue
            var bottom = h
            for (status in PortStatus.entries) {
                val n = counts[status.ordinal]
                if (n == 0) continue
                val segment = max(h * n / size, minSliver)
                val top = max(bottom - segment, 0f)
                canvas.drawRect(i * colW, top, (i + 1) * colW, bottom, paints[status.ordinal])
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
    }
}
