package com.vorsk.portquiz

import android.content.Context
import android.util.AttributeSet
import android.widget.ScrollView

/**
 * Holds the scan settings: as tall as its content, but at most
 * [MAX_FRACTION] of the screen, scrolling beyond that, so the results below
 * always keep room.
 */
class SettingsScrollView(context: Context, attrs: AttributeSet?) : ScrollView(context, attrs) {
    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        val max = (resources.displayMetrics.heightPixels * MAX_FRACTION).toInt()
        val available = MeasureSpec.getSize(heightMeasureSpec)
        val limit = if (MeasureSpec.getMode(heightMeasureSpec) == MeasureSpec.UNSPECIFIED) max else minOf(max, available)
        super.onMeasure(widthMeasureSpec, MeasureSpec.makeMeasureSpec(limit, MeasureSpec.AT_MOST))
    }

    private companion object {
        const val MAX_FRACTION = 0.5
    }
}
