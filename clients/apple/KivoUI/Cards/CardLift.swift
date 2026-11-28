import SwiftUI

public extension View {
    /// A card's artwork, which on the Apple TV lifts and catches the light
    /// while its button has focus, as the system's lockups do, with room
    /// under it to grow into; the caption under it stays put. Elsewhere
    /// it's as it is.
    @ViewBuilder func cardLift() -> some View {
        #if os(tvOS)
        hoverEffect(.highlight)
            .padding(.bottom, cardLiftRoom)
        #else
        self
        #endif
    }
}

#if os(tvOS)
/// The room under a card's artwork for its lift, which with the card's own
/// gap over its caption leaves the caption 32 points under the artwork.
let cardLiftRoom = KivoTokens.spacing * 7
#endif
