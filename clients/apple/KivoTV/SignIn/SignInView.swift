import KivoCore
import KivoUI
import SwiftUI

/// Signing in on the TV: the server signed in to last, reached first, then
/// the servers found on the network, choosing one found by itself, with
/// Enter address a step away; then the sign-in code, which a signed-in
/// device approves, with the password as the alternative. A server the app
/// doesn't work with says which of them needs an update, in place of its
/// code. Menu goes back a step.
struct SignInView: View {
    let saved: SavedServer?
    @Environment(AppModel.self) private var app
    @State private var model: SignInModel?
    /// Typing the server's address, in place of the servers found.
    @State private var enteringAddress = false
    /// Signing in with a password, in place of the code.
    @State private var usingPassword = false

    var body: some View {
        Group {
            if let model {
                switch model.step {
                case .findServer where enteringAddress:
                    AddressForm(model: model) { enteringAddress = false }
                case .findServer:
                    FindServer(model: model) { enteringAddress = true }
                case let .server(found) where usingPassword:
                    PasswordForm(model: model, found: found) { usingPassword = false }
                case let .server(found):
                    LinkCodePanel(found: found, usePassword: { usingPassword = true }, changeServer: changeServer)
                        .id(found.info.id)
                case let .incompatible(_, problem):
                    CompatibilityView(problem: problem, retry: model.retryCompatibility, changeServer: changeServer)
                        .onExitCommand(perform: changeServer)
                }
            } else {
                ProgressView()
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .task {
            let model = model ?? SignInModel(app: app, saved: saved)
            self.model = model
            if saved != nil { await model.chooseSaved() }
            if case .findServer = model.step { await model.discover() }
        }
        .onDisappear { model?.stopDiscovering() }
    }

    private func changeServer() {
        usingPassword = false
        enteringAddress = false
        model?.back()
    }
}

/// A step of sign-in: Kivo's logo over its title, and what the step shows.
private struct SignInStep<Content: View>: View {
    let title: String
    var detail: String?
    @ViewBuilder let content: () -> Content
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 8) {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
                KivoLockup()
                Text(title)
                    .kivoStyle(KivoTokens.TextSize.xl2, weight: KivoTokens.Weight.bold)
                    .foregroundStyle(palette.text)
                if let detail {
                    Text(detail)
                        .kivoStyle(KivoTokens.TextSize.base)
                        .foregroundStyle(palette.textMuted)
                }
            }
            content()
        }
        .frame(width: 900, alignment: .leading)
    }
}

/// Kivo's logo: the Facet K beside "Kivo".
private struct KivoLockup: View {
    var body: some View {
        Image("Logo")
            .resizable()
            .aspectRatio(contentMode: .fit)
            .frame(height: 49)
            .accessibilityLabel(Text("Kivo"))
    }
}

/// The servers found on the network, the first focused, each with its
/// address, and Enter address; while it looks, it says so.
private struct FindServer: View {
    let model: SignInModel
    let enterAddress: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        SignInStep(title: String(localized: "Sign in"), detail: status) {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
                if let saved = model.saved, !model.discovered.contains(where: { $0.info.id == saved.id }) {
                    Button(saved.name) { Task { await model.chooseSaved() } }
                }
                ForEach(model.discovered, id: \.info.id) { found in
                    Button {
                        model.choose(found)
                    } label: {
                        Text("\(found.info.name) · \(Self.address(found.baseUrl))")
                    }
                }
                Button(String(localized: "Enter address"), action: enterAddress)
                if let message = model.message {
                    Text(message)
                        .kivoStyle(KivoTokens.TextSize.base)
                        .foregroundStyle(palette.danger)
                }
            }
            .disabled(model.busy)
        }
    }

    private var status: String {
        if model.discovering { return String(localized: "Looking for servers on this network…") }
        if model.discovered.isEmpty { return String(localized: "No servers found. Enter the address, or find it on a signed-in phone under Settings › Link a device.") }
        return String(localized: "Choose a server.")
    }

    /// An address without its scheme, as people read it.
    private static func address(_ url: String) -> String {
        url.replacingOccurrences(of: "https://", with: "").replacingOccurrences(of: "http://", with: "")
    }
}

/// The server's address, typed with the TV's keyboard, then Connect.
private struct AddressForm: View {
    @Bindable var model: SignInModel
    let back: () -> Void
    @Environment(\.kivoPalette) private var palette
    @FocusState private var focused: Bool

    var body: some View {
        SignInStep(title: String(localized: "Enter address")) {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
                TextField(String(localized: "Server address"), text: $model.address)
                    .textContentType(.URL)
                    .keyboardType(.URL)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .focused($focused)
                    .onSubmit { Task { await model.connect() } }
                Text("Such as 192.168.1.20, nas:8484 or https://kivo.example.com")
                    .kivoStyle(KivoTokens.TextSize.sm)
                    .foregroundStyle(palette.textMuted)
                if let message = model.message {
                    Text(message)
                        .kivoStyle(KivoTokens.TextSize.base)
                        .foregroundStyle(palette.danger)
                }
                HStack(spacing: KivoTokens.spacing * 4) {
                    Button(model.busy ? String(localized: "Connecting…") : String(localized: "Connect")) {
                        Task { await model.connect() }
                    }
                    .disabled(model.address.isEmpty || model.busy)
                    Button(String(localized: "Back"), action: back)
                }
            }
        }
        .onExitCommand(perform: back)
        .onAppear { focused = true }
    }
}

/// The sign-in code and its QR code, for a signed-in device to approve,
/// with Sign in with password, focused first, and Change server. An
/// expired code gives way to a new one; a declined one says so over its
/// replacement; once one is approved, the app is signed in.
private struct LinkCodePanel: View {
    let found: FoundServer
    let usePassword: () -> Void
    let changeServer: () -> Void
    @Environment(AppModel.self) private var app
    @Environment(\.kivoPalette) private var palette
    @State private var waiting: LinkProgressWaiting?
    @State private var declined = false
    @State private var failure: RequestProblem?
    /// Counts up to start again after a failure.
    @State private var attempt = 0

    var body: some View {
        Group {
            if let failure {
                MessageView(
                    String(localized: "Couldn't get a sign-in code."),
                    sentence: failure.sentence,
                    action: (String(localized: "Try again"), { attempt += 1 }),
                    secondary: (String(localized: "Sign in with password"), usePassword)
                )
            } else {
                HStack(alignment: .center, spacing: KivoTokens.spacing * 28) {
                    SignInStep(title: String(localized: "Sign in"), detail: String(localized: "On a signed-in device, scan the QR code, or open Link a device and enter this code.")) {
                        VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
                            if declined {
                                Text("Sign-in was declined.")
                                    .kivoStyle(KivoTokens.TextSize.base)
                                    .foregroundStyle(palette.danger)
                            }
                            Text(waiting.map { groupedCode($0.code) } ?? " ")
                                .font(.system(size: KivoTokens.TextSize.xl3.size, weight: .semibold, design: .monospaced))
                                .foregroundStyle(palette.text)
                                .accessibilityIdentifier("link-code")
                                // Read a character at a time; the label stays the bare code.
                                .accessibilityLabel(Text(verbatim: waiting?.code ?? "").speechSpellsOutCharacters())
                            Text(found.info.name)
                                .kivoStyle(KivoTokens.TextSize.sm)
                                .foregroundStyle(palette.textSubtle)
                            HStack(spacing: KivoTokens.spacing * 4) {
                                Button(String(localized: "Sign in with password"), action: usePassword)
                                Button(String(localized: "Change server"), action: changeServer)
                            }
                            .padding(.top, KivoTokens.spacing * 4)
                        }
                    }
                    .frame(width: 900)
                    Group {
                        if let waiting {
                            QRCode(text: waiting.linkUrl)
                        } else {
                            RoundedRectangle(cornerRadius: KivoTokens.Radius.lg).fill(palette.surfaceRaised)
                        }
                    }
                    .frame(width: 448, height: 448)
                }
            }
        }
        .onExitCommand(perform: changeServer)
        .task(id: attempt) { await link() }
    }

    /// Follows codes until one is approved, or the link fails.
    private func link() async {
        failure = nil
        while !Task.isCancelled {
            for await progress in app.core.signInByLink(found: found, name: AppleIdentity.modelName) {
                switch onEnum(of: progress) {
                case let .waiting(next):
                    waiting = next
                case let .signedIn(signedIn):
                    app.core.signedIn(session: signedIn.session)
                    return
                case .denied:
                    declined = true
                case .expired:
                    break
                case let .failed(failed):
                    failure = failed.problem
                    return
                }
            }
        }
    }
}

/// Signing in with a password, a step at a time, each field with the
/// keyboard to itself: the username, then Next, then the password, which
/// names the username it goes with. Menu from the password returns to the
/// username, kept as typed, and from the username to the code.
private struct PasswordForm: View {
    @Bindable var model: SignInModel
    let found: FoundServer
    let back: () -> Void
    @Environment(\.kivoPalette) private var palette
    @State private var askingPassword = false
    @FocusState private var focused: Bool

    var body: some View {
        SignInStep(
            title: String(localized: "Sign in to \(found.info.name)"),
            detail: askingPassword ? String(localized: "As \(model.username.trimmingCharacters(in: .whitespaces))") : nil
        ) {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
                if askingPassword {
                    SecureField(String(localized: "Password"), text: $model.password)
                        .textContentType(.password)
                        .focused($focused)
                        .onSubmit { Task { await model.signIn(to: found) } }
                } else {
                    TextField(String(localized: "Username"), text: $model.username)
                        .textContentType(.username)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .focused($focused)
                        .onSubmit(next)
                }
                if let message = model.message {
                    Text(message)
                        .kivoStyle(KivoTokens.TextSize.base)
                        .foregroundStyle(palette.danger)
                }
                HStack(spacing: KivoTokens.spacing * 4) {
                    if askingPassword {
                        Button(model.busy ? String(localized: "Signing in…") : String(localized: "Sign in")) {
                            Task { await model.signIn(to: found) }
                        }
                        .disabled(model.password.isEmpty || model.busy)
                    } else {
                        Button(String(localized: "Next"), action: next)
                            .disabled(model.username.trimmingCharacters(in: .whitespaces).isEmpty)
                    }
                    Button(String(localized: "Back"), action: stepBack)
                }
            }
        }
        .onExitCommand(perform: stepBack)
        .onAppear { focused = true }
        .onChange(of: askingPassword) { focused = true }
    }

    private func next() {
        guard !model.username.trimmingCharacters(in: .whitespaces).isEmpty else { return }
        askingPassword = true
    }

    private func stepBack() {
        if askingPassword {
            askingPassword = false
            model.password = ""
        } else {
            back()
        }
    }
}
