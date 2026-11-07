package com.mmcdole.kivo.android

import kotlin.test.Test
import kotlin.test.assertEquals

class UiChoiceTest {
    @Test
    fun aTelevisionShowsTheTvInterface() {
        assertEquals(Ui.Tv, chooseUi(television = true, leanback = true))
    }

    @Test
    fun eitherSignAloneIsEnough() {
        assertEquals(Ui.Tv, chooseUi(television = true, leanback = false))
        assertEquals(Ui.Tv, chooseUi(television = false, leanback = true))
    }

    @Test
    fun anythingElseShowsThePhoneInterface() {
        assertEquals(Ui.Mobile, chooseUi(television = false, leanback = false))
    }
}
