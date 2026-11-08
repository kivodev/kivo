package com.mmcdole.kivo.android.mobile.app

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.PageTopBar

/** Where Play goes until the player comes. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PlayerPlaceholder(nav: MobileNav) {
    Scaffold(topBar = { PageTopBar("", nav::back) }) { padding ->
        Message(stringResource(R.string.mobile_player_coming), null, Modifier.padding(padding).testTag("playerPlaceholder"))
    }
}
