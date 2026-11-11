import KivoUI
import SwiftUI

/// Account: Change password…, which opens a sheet, Signed-in devices,
/// Change server, which leaves for sign-in with the device still signed in,
/// and Sign out, which asks first; [signedOut] runs as the sign-out starts.
struct AccountSettingsView: View {
    let signedOut: () -> Void
    @Environment(AppModel.self) private var app
    @State private var changingPassword = false
    @State private var signingOut = false

    var body: some View {
        Form {
            Section {
                Button {
                    changingPassword = true
                } label: {
                    Text("Change password…")
                }
                .foregroundStyle(.primary)
                NavigationLink { SessionsView() } label: {
                    Text(String(localized: "Signed-in devices"))
                }
            }
            Section {
                Button(String(localized: "Change server")) { app.core.changeServer() }
                    .foregroundStyle(.primary)
                    .accessibilityIdentifier("changeServer")
            }
            Section {
                Button(String(localized: "Sign out"), role: .destructive) { signingOut = true }
                    .frame(maxWidth: .infinity)
                    .signOutConfirmation(isPresented: $signingOut, signedOut: signedOut)
            }
        }
        .navigationTitle(String(localized: "Account"))
        .toolbarTitleDisplayMode(.inline)
        .sheet(isPresented: $changingPassword) {
            NavigationStack { ChangePasswordView() }
        }
    }
}
