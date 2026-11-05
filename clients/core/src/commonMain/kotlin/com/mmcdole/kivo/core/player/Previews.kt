package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.PreviewManifest
import com.mmcdole.kivo.core.resolveUrl

/** Where a position's preview frame is: its sheet, and the frame's place in it, in pixels. */
data class SpriteFrame(val sheetUrl: String, val x: Int, val y: Int, val width: Int, val height: Int) {
    companion object {
        /**
         * The scrub-preview frame for a position, as the web player finds it: sheets
         * of `columns` by `rows` frames, one every `intervalMs`, the last held past
         * the end. Sheet URLs are resolved against the manifest's own URL.
         */
        fun atOrNull(manifest: PreviewManifest, manifestUrl: String, positionMs: Long): SpriteFrame? {
            val perSheet = manifest.columns * manifest.rows
            if (perSheet <= 0 || manifest.intervalMs <= 0 || manifest.sheets.isEmpty()) return null
            val frame = minOf(maxOf(0L, positionMs / manifest.intervalMs), manifest.sheets.size.toLong() * perSheet - 1).toInt()
            val within = frame % perSheet
            return SpriteFrame(
                sheetUrl = resolveUrl(manifestUrl, manifest.sheets[frame / perSheet]),
                x = (within % manifest.columns) * manifest.width,
                y = (within / manifest.columns) * manifest.height,
                width = manifest.width,
                height = manifest.height,
            )
        }
    }
}
