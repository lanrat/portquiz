package com.vorsk.portquiz

import android.content.ContentProvider
import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.provider.OpenableColumns
import java.io.File
import java.io.FileNotFoundException

/**
 * Serves saved JSON reports to the app a report is shared with. Android
 * limits how much an intent can carry to about 1 MB, which a full scan's
 * report can exceed, so reports are shared as files rather than as text.
 * Other apps can only read a report they were granted access to.
 */
class ReportProvider : ContentProvider() {
    override fun onCreate() = true

    override fun getType(uri: Uri) = TYPE

    override fun openFile(uri: Uri, mode: String): ParcelFileDescriptor {
        if (mode != "r") throw SecurityException("reports are read only")
        val file = fileFor(uri) ?: throw FileNotFoundException(uri.toString())
        return ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
    }

    // the name and size, which receiving apps use to label the attachment
    override fun query(
        uri: Uri,
        projection: Array<out String>?,
        selection: String?,
        selectionArgs: Array<out String>?,
        sortOrder: String?,
    ): Cursor? {
        val file = fileFor(uri) ?: return null
        val columns = projection ?: arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE)
        val row = columns.map {
            when (it) {
                OpenableColumns.DISPLAY_NAME -> file.name
                OpenableColumns.SIZE -> file.length()
                else -> null
            }
        }
        return MatrixCursor(columns, 1).apply { addRow(row) }
    }

    override fun insert(uri: Uri, values: ContentValues?): Uri? = null

    override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?) = 0

    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?) = 0

    /** The report a URI names, if it exists and is in the reports directory. */
    private fun fileFor(uri: Uri): File? {
        val dir = dir(context ?: return null).canonicalFile
        val name = uri.lastPathSegment ?: return null
        val file = File(dir, name).canonicalFile
        return file.takeIf { it.parentFile == dir && it.isFile }
    }

    companion object {
        const val TYPE = "application/json"
        private const val AUTHORITY = BuildConfig.APPLICATION_ID + ".reports"

        /** Where reports to share are saved. */
        fun dir(context: Context) = File(context.cacheDir, "reports")

        /**
         * Saves json as the only report to share, replacing any shared
         * before, and returns its URI.
         */
        fun save(context: Context, name: String, json: String): Uri {
            val dir = dir(context)
            dir.mkdirs()
            dir.listFiles()?.forEach { it.delete() }
            File(dir, name).writeText(json)
            return Uri.Builder().scheme("content").authority(AUTHORITY).appendPath(name).build()
        }
    }
}
