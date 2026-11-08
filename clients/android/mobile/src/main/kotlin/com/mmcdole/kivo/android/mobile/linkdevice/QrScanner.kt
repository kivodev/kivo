package com.mmcdole.kivo.android.mobile.linkdevice

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.ImageProxy
import androidx.camera.view.CameraController
import androidx.camera.view.LifecycleCameraController
import androidx.camera.view.PreviewView
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.google.zxing.BinaryBitmap
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.ReaderException
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.asExecutor

/** Whether there's a camera to scan with. */
fun hasCamera(context: Context): Boolean = context.packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)

/**
 * The camera, reading QR codes, until one is read. It asks for the camera
 * permission as it opens, and stays black without it. Holding the camera
 * on a code reads it once.
 */
@Composable
fun QrScanner(onScanned: (String) -> Unit, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val owner = LocalLifecycleOwner.current
    val scanned by rememberUpdatedState(onScanned)
    var granted by remember { mutableStateOf(ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) }
    val ask = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted = it }
    LaunchedEffect(Unit) { if (!granted) ask.launch(Manifest.permission.CAMERA) }
    val controller = remember { LifecycleCameraController(context).apply { setEnabledUseCases(CameraController.IMAGE_ANALYSIS) } }
    DisposableEffect(granted, owner) {
        if (granted) {
            val main = ContextCompat.getMainExecutor(context)
            var last: String? = null
            controller.setImageAnalysisAnalyzer(Dispatchers.Default.asExecutor()) { image ->
                val code = image.use(::readQrCode) ?: return@setImageAnalysisAnalyzer
                main.execute {
                    if (code != last) {
                        last = code
                        scanned(code)
                    }
                }
            }
            controller.bindToLifecycle(owner)
        }
        onDispose {
            controller.unbind()
            controller.clearImageAnalysisAnalyzer()
        }
    }
    Box(modifier.background(Color.Black)) {
        if (granted) {
            AndroidView(
                factory = { PreviewView(it).apply { this.controller = controller } },
                modifier = Modifier.fillMaxSize(),
            )
        }
    }
}

/** The text of the QR code in a camera frame, from its luminance; null when it shows none. */
private fun readQrCode(image: ImageProxy): String? {
    val plane = image.planes[0]
    val buffer = plane.buffer
    val luminance = ByteArray(plane.rowStride * image.height)
    buffer.get(luminance, 0, minOf(buffer.remaining(), luminance.size))
    return readQrCode(luminance, plane.rowStride, image.width, image.height)
}

/** The text of the QR code in [luminance], [width] by [height] with rows [rowStride] apart; null when it shows none. */
internal fun readQrCode(luminance: ByteArray, rowStride: Int, width: Int, height: Int): String? {
    val source = PlanarYUVLuminanceSource(luminance, rowStride, height, 0, 0, width, height, false)
    return try {
        QRCodeReader().decode(BinaryBitmap(HybridBinarizer(source))).text
    } catch (_: ReaderException) {
        null
    }
}
