import KivoCore
import KivoUI
import SwiftUI

/// Signed-in devices: Link a device, then the user's sessions, one for each
/// browser and app the user is signed in on, with what each runs on, most
/// recently used first, each with Sign out,
/// and under them Sign out everywhere else. Every sign-out asks first;
/// signing out this one goes to sign-in. A failure says why where it
/// happened, with Retry.
struct SessionsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var sessions: [SignedInDevice]?
    @State private var loadFailure: String?
    /// The session whose sign-out failed, and why.
    @State private var signOutFailure: (id: String, message: String)?
    @State private var everywhereElseFailure: String?
    @State private var confirmingEverywhereElse = false
    @State private var confirmingSignOut: SignedInDevice?
    @State private var signingOutHere = false

    var body: some View {
        List {
            Section {
                NavigationLink { LinkDeviceView() } label: {
                    Text(String(localized: "Link a device"))
                }
            }
            if let sessions {
                Section {
                    ForEach(sessions, id: \.id) { session in
                        row(session)
                    }
                }
                if sessions.contains(where: { !$0.current }) {
                    Section {
                        Button(String(localized: "Sign out everywhere else"), role: .destructive) {
                            confirmingEverywhereElse = true
                        }
                        .accessibilityIdentifier("sign-out-everywhere-else")
                        if let everywhereElseFailure {
                            FailureRow(message: everywhereElseFailure) { Task { await signOutEverywhereElse() } }
                        }
                    }
                }
            } else if let loadFailure {
                FailureRow(message: loadFailure) { Task { await load() } }
            } else {
                ProgressView().frame(maxWidth: .infinity)
            }
        }
        .navigationTitle(String(localized: "Signed-in devices"))
        .toolbarTitleDisplayMode(.inline)
        .confirmationDialog(String(localized: "Sign out everywhere else?"), isPresented: $confirmingEverywhereElse, titleVisibility: .visible) {
            Button(String(localized: "Sign out everywhere else"), role: .destructive) {
                Task { await signOutEverywhereElse() }
            }
        } message: {
            Text(ThisDevice.staysSignedIn)
        }
        .confirmationDialog(
            String(localized: "Sign out \(confirmingSignOut.map(name) ?? "")?"),
            isPresented: Binding(get: { confirmingSignOut != nil }, set: { if !$0 { confirmingSignOut = nil } }),
            titleVisibility: .visible,
            presenting: confirmingSignOut
        ) { session in
            Button(String(localized: "Sign out"), role: .destructive) {
                Task { await signOut(session) }
            }
        } message: { session in
            Text("\(name(session)) has to sign in again.")
        }
        .signOutConfirmation(isPresented: $signingOutHere)
        .task { await load() }
        .refreshable { await load() }
    }

    private func row(_ session: SignedInDevice) -> some View {
        VStack(alignment: .leading) {
            HStack {
                VStack(alignment: .leading) {
                    Text(name(session))
                    Text(subtitle(session))
                        .font(.footnote)
                        .foregroundStyle(palette.textMuted)
                }
                Spacer()
                Button(String(localized: "Sign out"), role: .destructive) {
                    if session.current { signingOutHere = true } else { confirmingSignOut = session }
                }
                .buttonStyle(.borderless)
                .accessibilityLabel(Text("Sign out \(name(session))"))
            }
            if let signOutFailure, signOutFailure.id == session.id {
                FailureRow(message: signOutFailure.message) { Task { await signOut(session) } }
            }
        }
    }

    /// The session's name, or what it is when it has none, as a browser may not.
    private func name(_ session: SignedInDevice) -> String {
        guard session.name.isEmpty else { return session.name }
        return session.kind == "browser" ? String(localized: "Web browser") : String(localized: "App")
    }

    private func subtitle(_ session: SignedInDevice) -> String {
        let kind = Platform.name(session.platform) ?? (session.kind == "browser" ? String(localized: "Browser") : String(localized: "App"))
        if session.current { return "\(kind) · \(ThisDevice.name)" }
        let lastUsed = Date(timeIntervalSince1970: TimeInterval(session.lastUsedEpochMs) / 1000)
        return String(localized: "\(kind) · Last used \(lastUsed.formatted(.relative(presentation: .named)))")
    }

    private func load() async {
        do {
            sessions = try await server.session.account.sessions()
            loadFailure = nil
        } catch {
            loadFailure = error.problemSentence
        }
    }

    private func signOutEverywhereElse() async {
        do {
            try await server.session.account.signOutEverywhereElse()
            everywhereElseFailure = nil
            await load()
        } catch {
            everywhereElseFailure = error.problemSentence
        }
    }

    private func signOut(_ session: SignedInDevice) async {
        do {
            try await server.session.account.signOutSession(id: session.id)
            signOutFailure = nil
            await load()
        } catch {
            signOutFailure = (session.id, error.problemSentence)
        }
    }
}
