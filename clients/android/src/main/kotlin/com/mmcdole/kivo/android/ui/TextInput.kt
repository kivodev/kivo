package com.mmcdole.kivo.android.ui

import android.view.KeyEvent
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.PlatformImeOptions
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType

/**
 * A text field with its label above it, as in a form, typed
 * with the platform keyboard, which opens when it has focus. The remote
 * never gets stuck in it: Up and Down always move on, and Left and Right
 * do at either end of the text. Next on the keyboard moves down, or runs
 * [onNext] when given, as a form step's does. [keyboardSide] asks the
 * keyboard to sit at that side of the screen, clear of a form beside it,
 * where the keyboard can; it's centred otherwise. [capitalization] sets
 * the keyboard's case, such as capitals for a code.
 */
@Composable
fun TextInput(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    password: Boolean = false,
    keyboardType: KeyboardType = KeyboardType.Text,
    imeAction: ImeAction = ImeAction.Next,
    onDone: () -> Unit = {},
    onNext: (() -> Unit)? = null,
    keyboardSide: Alignment.Horizontal? = null,
    capitalization: KeyboardCapitalization = KeyboardCapitalization.None,
    labelShown: Boolean = true,
    icon: KivoIcon? = null,
    minHeight: Dp = Dp.Unspecified,
) {
    val colors = Kivo.colors
    val focusManager = LocalFocusManager.current
    var focused by remember { mutableStateOf(false) }
    var field by remember { mutableStateOf(TextFieldValue(value, TextRange(value.length))) }
    if (field.text != value) field = TextFieldValue(value, TextRange(value.length))
    Column(modifier.fillMaxWidth()) {
        if (labelShown) Text(label, style = TvType.small, color = colors.textMuted, modifier = Modifier.padding(bottom = Kivo.space(1)))
        // Focused, the field alone is lit, white with dark text, as a control is; its label stays as it is.
        val shown = if (focused) KivoTokens.light else colors
        Box(
            Modifier
                .fillMaxWidth()
                .heightIn(min = minHeight)
                .background(shown.surfaceRaised, RoundedCornerShape(KivoTokens.Radius.md))
                .border(KivoTokens.Layout.focusRingWidth / 2, if (focused) Color.Transparent else colors.border, RoundedCornerShape(KivoTokens.Radius.md))
                .padding(start = if (icon != null) 40.dp else Kivo.space(3), end = Kivo.space(3), top = Kivo.space(2), bottom = Kivo.space(2)),
            contentAlignment = Alignment.CenterStart,
        ) {
            if (icon != null) {
                KivoIconView(icon, Modifier.align(Alignment.CenterStart).offset(x = (-28).dp), size = 20.dp, tint = shown.textMuted)
            }
            BasicTextField(
                value = field,
                onValueChange = {
                    field = it
                    if (it.text != value) onValueChange(it.text)
                },
                singleLine = true,
                textStyle = TvType.body.copy(color = shown.text),
                cursorBrush = SolidColor(shown.accent),
                visualTransformation = if (password) PasswordVisualTransformation() else VisualTransformation.None,
                keyboardOptions = KeyboardOptions(
                    capitalization = capitalization,
                    keyboardType = if (password) KeyboardType.Password else keyboardType,
                    imeAction = imeAction,
                    autoCorrectEnabled = false,
                    platformImeOptions = keyboardSide?.let { PlatformImeOptions("horizontalAlignment=${imeSide(it)}") },
                ),
                keyboardActions = KeyboardActions(onDone = { onDone() }, onGo = { onDone() }, onNext = { onNext?.invoke() ?: focusManager.moveFocus(FocusDirection.Down) }),
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("field:$label")
                    .onFocusChanged { focused = it.isFocused }
                    .onPreviewKeyEvent { event ->
                        if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                        val cursor = field.selection
                        val direction = when (event.key.nativeKeyCode) {
                            KeyEvent.KEYCODE_DPAD_UP -> FocusDirection.Up
                            KeyEvent.KEYCODE_DPAD_DOWN -> FocusDirection.Down
                            KeyEvent.KEYCODE_DPAD_LEFT -> FocusDirection.Left.takeIf { cursor.collapsed && cursor.start == 0 }
                            KeyEvent.KEYCODE_DPAD_RIGHT -> FocusDirection.Right.takeIf { cursor.collapsed && cursor.end == field.text.length }
                            else -> null
                        } ?: return@onPreviewKeyEvent false
                        focusManager.moveFocus(direction)
                    },
            )
        }
    }
}

/** The on-screen keyboard's name for a side of the screen. */
private fun imeSide(side: Alignment.Horizontal) = when (side) {
    Alignment.Start -> "left"
    Alignment.End -> "right"
    else -> "center"
}
