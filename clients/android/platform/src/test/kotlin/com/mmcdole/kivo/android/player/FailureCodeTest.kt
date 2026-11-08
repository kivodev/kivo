package com.mmcdole.kivo.android.player

import androidx.annotation.OptIn
import androidx.media3.common.PlaybackException
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.DataSpec
import androidx.media3.datasource.HttpDataSource
import java.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import sun.misc.Unsafe

@OptIn(UnstableApi::class)
class FailureCodeTest {
    /** The request, made without its constructor, which needs a Uri the unit tests' Android classes can't make. */
    private val request = Unsafe::class.java.getDeclaredField("theUnsafe").run {
        isAccessible = true
        (get(null) as Unsafe).allocateInstance(DataSpec::class.java) as DataSpec
    }

    private fun status(code: Int) =
        HttpDataSource.InvalidResponseCodeException(code, null, null, emptyMap(), request, ByteArray(0))

    @Test
    fun a500FallsBackRatherThanReconnecting() {
        assertEquals("mediaSourceError", failureCode(PlaybackException.ERROR_CODE_IO_BAD_HTTP_STATUS, status(500)))
    }

    @Test
    fun otherStatusesAndNetworkErrorsStayNetworkErrors() {
        for (code in listOf(404, 502, 503, 504)) {
            assertEquals("networkError", failureCode(PlaybackException.ERROR_CODE_IO_BAD_HTTP_STATUS, status(code)))
        }
        assertEquals("networkError", failureCode(PlaybackException.ERROR_CODE_IO_NETWORK_CONNECTION_FAILED, IOException()))
        assertEquals("networkError", failureCode(PlaybackException.ERROR_CODE_IO_BAD_HTTP_STATUS, null))
    }
}
