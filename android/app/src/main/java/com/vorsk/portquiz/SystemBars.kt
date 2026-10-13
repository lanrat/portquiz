package com.vorsk.portquiz

import android.app.Activity
import android.os.Build
import android.view.View
import android.view.WindowInsets

/**
 * Draws behind the status and navigation bars on every Android version,
 * as Android 15 and later require, so [applyInsets] works the same way
 * everywhere.
 */
fun Activity.drawBehindSystemBars() {
    when {
        // Android 15 and later always draw behind the system bars
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM -> {}
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.R ->
            @Suppress("DEPRECATION")
            window.setDecorFitsSystemWindows(false)
        else ->
            @Suppress("DEPRECATION")
            window.decorView.systemUiVisibility = window.decorView.systemUiVisibility or
                View.SYSTEM_UI_FLAG_LAYOUT_STABLE or
                View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or
                View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
    }
}

/** Keeps content clear of the status bar, navigation bar and keyboard. */
fun applyInsets(root: View) {
    val left = root.paddingLeft
    val top = root.paddingTop
    val right = root.paddingRight
    val bottom = root.paddingBottom
    root.setOnApplyWindowInsetsListener { v, insets ->
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            val bars = insets.getInsets(
                WindowInsets.Type.systemBars() or WindowInsets.Type.displayCutout() or WindowInsets.Type.ime()
            )
            v.setPadding(left + bars.left, top + bars.top, right + bars.right, bottom + bars.bottom)
        } else {
            @Suppress("DEPRECATION")
            v.setPadding(
                left + insets.systemWindowInsetLeft, top + insets.systemWindowInsetTop,
                right + insets.systemWindowInsetRight, bottom + insets.systemWindowInsetBottom,
            )
        }
        insets
    }
}
