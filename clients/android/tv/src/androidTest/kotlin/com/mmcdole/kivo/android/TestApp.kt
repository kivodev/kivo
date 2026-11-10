package com.mmcdole.kivo.android

import android.app.Application
import com.mmcdole.kivo.android.tv.test.R

/** The tests' application: the [AppGraph] the screens reach, with a short list of libraries in place of the app's build's. */
class TestApp : Application(), HasAppGraph {
    override val graph: AppGraph by lazy { AppGraph(this, AppBuild("test", "0.0.0+0", R.raw.test_libraries)) }
}
