import KivoCore
import KivoUI
import SwiftUI

/// Change password: the current password, a new one of at least 8
/// characters, and whether to sign out the user's other browsers and apps,
/// which starts on. This app stays signed in, and the sheet closes once the
/// server has changed it. A wrong current password says so under its
/// field; any other refusal says why, with Retry.
struct ChangePasswordView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    @State private var current = ""
    @State private var newPassword = ""
    @State private var signOutOthers = true
    @State private var changing = false
    @State private var wrongPassword = false
    @State private var failure: String?
    @FocusState private var field: Field?

    private enum Field {
        case current
        case new
    }

    private static let minimumLength = 8

    private var ready: Bool {
        !current.isEmpty && newPassword.count >= Self.minimumLength && !changing
    }

    var body: some View {
        Form {
            Section {
                SecureField(String(localized: "Current password"), text: $current)
                    .textContentType(.password)
                    .focused($field, equals: .current)
                    .submitLabel(.next)
                    .onSubmit { field = .new }
                    .accessibilityIdentifier("current-password")
            } footer: {
                if wrongPassword {
                    Text("That isn't your current password.").foregroundStyle(palette.danger)
                }
            }
            Section {
                SecureField(String(localized: "New password"), text: $newPassword)
                    .textContentType(.newPassword)
                    .focused($field, equals: .new)
                    .submitLabel(.done)
                    .onSubmit { if ready { Task { await change() } } }
                    .accessibilityIdentifier("new-password")
            } footer: {
                Text("At least 8 characters.")
            }
            Section {
                Toggle(String(localized: "Sign out other devices"), isOn: $signOutOthers)
            }
            if let failure {
                Section {
                    FailureRow(message: failure) { Task { await change() } }
                }
            }
        }
        .navigationTitle(String(localized: "Change password"))
        .toolbarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button(String(localized: "Cancel")) { dismiss() }
            }
            ToolbarItem(placement: .confirmationAction) {
                if changing {
                    ProgressView()
                } else {
                    Button(String(localized: "Change")) { Task { await change() } }
                        .disabled(!ready)
                }
            }
        }
        .onChange(of: current) { wrongPassword = false }
        .interactiveDismissDisabled(!current.isEmpty || !newPassword.isEmpty)
        .onAppear { field = .current }
    }

    private func change() async {
        changing = true
        defer { changing = false }
        do {
            try await server.session.account.changePassword(current: current, password: newPassword, signOutOthers: signOutOthers)
            dismiss()
        } catch {
            failure = nil
            wrongPassword = false
            // A wrong current password is the server's refusal of the password, never of the session.
            if error.problemCode == "wrongPassword" || error.problemCode == "invalidCredentials" || error.problemStatus == 401 {
                wrongPassword = true
                field = .current
            } else {
                failure = error.problemSentence
            }
        }
    }
}
