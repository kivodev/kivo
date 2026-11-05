package com.mmcdole.kivo.core.downloads

/**
 * The device's storage for downloads, as the platform measures it: a
 * folder only downloads use, kept out of backups, the space free beside
 * it, and a file's hash.
 */
internal interface DownloadDisk {
    /** The folder downloads are kept in, an absolute path. */
    val folder: String

    /** Bytes free for downloads on the volume holding [folder]. */
    fun freeBytes(): Long

    /** A file's SHA-256, in lowercase hex. */
    fun sha256(path: String): String
}
