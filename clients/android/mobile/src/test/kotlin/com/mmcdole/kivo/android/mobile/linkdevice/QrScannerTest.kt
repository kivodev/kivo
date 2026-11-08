package com.mmcdole.kivo.android.mobile.linkdevice

import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class QrScannerTest {
    /** A camera frame's luminance showing [text]'s QR code, dark on light, with rows padded past its width as a camera's are. */
    private fun frame(text: String, size: Int, rowStride: Int): ByteArray {
        val matrix = QRCodeWriter().encode(text, BarcodeFormat.QR_CODE, size, size)
        val luminance = ByteArray(rowStride * size)
        for (y in 0 until size) {
            for (x in 0 until size) luminance[y * rowStride + x] = if (matrix[x, y]) 20 else 230.toByte()
        }
        return luminance
    }

    @Test
    fun aFrameShowingATvsQrCodeReadsItsLink() {
        val link = "https://kivo.test/link?code=K7Q2M9&server=s1"
        assertEquals(link, readQrCode(frame(link, 480, rowStride = 512), rowStride = 512, width = 480, height = 480))
    }

    @Test
    fun aFrameShowingNoCodeReadsNothing() {
        assertNull(readQrCode(ByteArray(640 * 480) { 128.toByte() }, rowStride = 640, width = 640, height = 480))
    }
}
