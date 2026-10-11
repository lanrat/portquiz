package com.vorsk.portquiz

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.PowerManager
import android.os.SystemClock
import com.vorsk.portquiz.mobile.Listener
import com.vorsk.portquiz.mobile.Mobile
import com.vorsk.portquiz.mobile.Scan
import com.vorsk.portquiz.mobile.Summary
import kotlin.concurrent.thread

/**
 * Runs a scan as a foreground service, so it keeps going with the screen off
 * or the app in the background, and shows its progress in a notification.
 */
class ScanService : Service() {
    private val main = Handler(Looper.getMainLooper())
    private lateinit var notifications: NotificationManager
    private var scan: Scan? = null
    private var stopRequested = false
    private var wakeLock: PowerManager.WakeLock? = null
    private var lastNotified = 0L

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        notifications = getSystemService(NotificationManager::class.java)
        notifications.createNotificationChannel(
            NotificationChannel(CHANNEL_PROGRESS, getString(R.string.channel_progress), NotificationManager.IMPORTANCE_LOW)
        )
        notifications.createNotificationChannel(
            NotificationChannel(CHANNEL_RESULTS, getString(R.string.channel_results), NotificationManager.IMPORTANCE_LOW)
        )
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> start()
            ACTION_STOP -> {
                // the scan may still be starting; it is canceled once it exists
                stopRequested = true
                scan?.cancel()
                if (!ScanState.running) stopSelf()
            }
        }
        return START_NOT_STICKY
    }

    private fun start() {
        if (ScanState.running) {
            // every startForegroundService call must be followed by
            // startForeground, even when a scan is already running
            startForeground(NOTIFICATION_PROGRESS, progressNotification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
            return
        }
        // MainActivity saves the settings before starting the scan
        val settings = Settings(this).load()
        val config = settings.toConfig()

        stopRequested = false
        ScanState.begin(settings.host)
        notifications.cancel(NOTIFICATION_RESULT)
        startForeground(NOTIFICATION_PROGRESS, progressNotification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        // keep the CPU running while the screen is off; the timeout is a
        // safety net, the lock is released when the scan finishes
        wakeLock = getSystemService(PowerManager::class.java)
            .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "portquiz:scan")
            .apply { acquire(6 * 60 * 60 * 1000L) }

        // starting resolves the host, which must not block the main thread
        thread(name = "portquiz-start") {
            try {
                val s = Mobile.start(config, listener)
                main.post {
                    scan = s
                    if (stopRequested) s.cancel()
                }
            } catch (e: Exception) {
                main.post { finish(canceled = false, missed = 0, error = e.message ?: e.toString()) }
            }
        }
    }

    /** Receives events from the Go scan on a background thread. */
    private val listener = object : Listener {
        override fun onRange(kind: String, first: Long, last: Long, open: Boolean) {
            main.post { ScanState.addRange(kind, first, last, open) }
        }

        override fun onProgress(done: Long, total: Long, open: Long, etaMillis: Long) {
            main.post {
                ScanState.progress(done, total, open, etaMillis)
                // Android drops notification updates that come too quickly
                val now = SystemClock.elapsedRealtime()
                if (now - lastNotified >= 1000) {
                    lastNotified = now
                    notifications.notify(NOTIFICATION_PROGRESS, progressNotification())
                }
            }
        }

        override fun onFinish(summary: Summary) {
            main.post {
                finish(summary.canceled, summary.missed, summary.error.ifEmpty { null })
            }
        }
    }

    private fun finish(canceled: Boolean, missed: Long, error: String?) {
        scan = null
        ScanState.finish(canceled, missed, error)
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
        stopForeground(STOP_FOREGROUND_REMOVE)
        notifications.notify(NOTIFICATION_RESULT, resultNotification())
        stopSelf()
    }

    // Android 15 limits how long a data sync service may run each day
    override fun onTimeout(startId: Int, fgsType: Int) {
        scan?.cancel()
    }

    override fun onDestroy() {
        scan?.cancel()
        wakeLock?.let { if (it.isHeld) it.release() }
        super.onDestroy()
    }

    private fun openAppIntent(): PendingIntent = PendingIntent.getActivity(
        this, 0,
        Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
        PendingIntent.FLAG_IMMUTABLE,
    )

    private fun progressNotification(): Notification {
        val stop = PendingIntent.getService(
            this, 0,
            Intent(this, ScanService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_IMMUTABLE,
        )
        val builder = Notification.Builder(this, CHANNEL_PROGRESS)
            .setSmallIcon(R.drawable.ic_port)
            .setContentTitle(getString(R.string.notification_title, ScanState.host))
            .setContentText(listOfNotNull(statusText(this), etaText(this)).joinToString(" · "))
            .setProgress(ScanState.total.toInt(), ScanState.done.toInt(), ScanState.total == 0L)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setContentIntent(openAppIntent())
            .addAction(Notification.Action.Builder(null, getString(R.string.stop), stop).build())
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            builder.setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE)
        }
        return builder.build()
    }

    private fun resultNotification(): Notification =
        Notification.Builder(this, CHANNEL_RESULTS)
            .setSmallIcon(R.drawable.ic_port)
            .setContentTitle(getString(R.string.notification_done_title, ScanState.host))
            .setContentText(statusText(this))
            .setContentIntent(openAppIntent())
            .setAutoCancel(true)
            .build()

    companion object {
        const val ACTION_START = "com.vorsk.portquiz.START"
        const val ACTION_STOP = "com.vorsk.portquiz.STOP"

        private const val CHANNEL_PROGRESS = "progress"
        private const val CHANNEL_RESULTS = "results"
        private const val NOTIFICATION_PROGRESS = 1
        private const val NOTIFICATION_RESULT = 2

        /** One line describing the scan's state, for the app and notifications. */
        fun statusText(context: Context): String = with(ScanState) {
            when {
                error != null -> context.getString(R.string.status_error, error)
                running && total == 0L -> context.getString(R.string.status_resolving, host)
                running -> context.getString(R.string.status_running, done, total, open)
                canceled -> context.getString(R.string.status_canceled, done, total, open)
                startedAt == 0L -> ""
                else -> context.getString(R.string.status_done, done, formatDuration(elapsed()), open)
            }
        }

        /** The estimated time left while a scan runs, or null. */
        fun etaText(context: Context): String? =
            ScanState.remaining()?.let { context.getString(R.string.status_eta, formatEta(roundEta(it))) }
    }
}
