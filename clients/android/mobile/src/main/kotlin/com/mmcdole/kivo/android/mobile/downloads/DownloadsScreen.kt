package com.mmcdole.kivo.android.mobile.downloads

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Scaffold
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.TabTopBar

/** Downloads' place, until downloads come. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DownloadsScreen() {
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = { TabTopBar(stringResource(R.string.mobile_tab_downloads), scroll) },
    ) { padding ->
        Message(stringResource(R.string.mobile_downloads_coming), stringResource(R.string.mobile_downloads_coming_sentence), Modifier.padding(padding))
    }
}
