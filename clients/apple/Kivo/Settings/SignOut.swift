import KivoCore
import SwiftUI
import UIKit

extension ThisDevice {
    private static var pad: Bool { UIDevice.current.userInterfaceIdiom == .pad }

    /// What signing this device out does to its downloads.
    static var downloadsDeleted: String {
        pad ? String(localized: "Downloads on this iPad are deleted.") : String(localized: "Downloads on this iPhone are deleted.")
    }

    /// Under "Sign out everywhere else?".
    static var staysSignedIn: String {
        pad
            ? String(localized: "Signs out all other browsers and apps. This iPad stays signed in.")
            : String(localized: "Signs out all other browsers and apps. This iPhone stays signed in.")
    }
}

extension View {
    /// Asks before signing this device out, saying that its downloads are
    /// deleted when it has any; [signedOut] runs as the sign-out starts.
    func signOutConfirmation(isPresented: Binding<Bool>, signedOut: @escaping () -> Void = {}) -> some View {
        modifier(SignOutConfirmation(isPresented: isPresented, signedOut: signedOut))
    }
}

private struct SignOutConfirmation: ViewModifier {
    @Binding var isPresented: Bool
    let signedOut: () -> Void
    @Environment(ServerModel.self) private var server

    func body(content: Content) -> some View {
        content.confirmationDialog(ThisDevice.signOutQuestion, isPresented: $isPresented, titleVisibility: .visible) {
            Button(String(localized: "Sign out"), role: .destructive) {
                signedOut()
                Task { await server.signOut() }
            }
        } message: {
            if !server.downloads.isEmpty {
                Text(ThisDevice.downloadsDeleted)
            }
        }
    }
}
