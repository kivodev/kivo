import KivoCore
import KivoUI
import SwiftUI

/// Sign-in: the server signed in to last, the servers Bonjour finds on the
/// network, "Enter address", and on a phone with a camera "Scan server QR
/// code"; then the password, with "Sign in with another device" under it.
/// On a wide screen the form keeps to a form's width in the middle. A
/// server the app doesn't work with says which of them needs an update, in
/// place of its password. A line under the last server, and over its
/// password, warns when its session ended as the server's identity changed.
struct SignInView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.kivoPalette) private var palette
    @State private var model: SignInModel?
    let saved: SavedServer?
    var identityChanged = false

    var body: some View {
        Group {
            if let model, case let .incompatible(_, problem) = model.step {
                CompatibilityView(problem: problem, retry: model.retryCompatibility, changeServer: model.back)
            } else {
                NavigationStack {
                    Group {
                        if let model {
                            switch model.step {
                            case .findServer, .incompatible: FindServerForm(model: model, identityChanged: identityChanged)
                            case let .password(found):
                                PasswordForm(model: model, found: found, identityChanged: identityChanged && found.info.id == saved?.id)
                            }
                        }
                    }
                    .scrollContentBackground(.hidden)
                    // A form's width on a wide screen, as on the web, rather than rows across it.
                    .frame(maxWidth: KivoTokens.Container.form)
                    .frame(maxWidth: .infinity)
                    .background(palette.bg)
                    .navigationTitle(String(localized: "Sign in"))
                }
            }
        }
        .task {
            if model == nil { model = SignInModel(app: app, saved: saved) }
            await model?.discover()
        }
        .onDisappear { model?.stopDiscovering() }
    }
}

private struct FindServerForm: View {
    @Bindable var model: SignInModel
    let identityChanged: Bool
    @Environment(\.kivoPalette) private var palette
    @State private var scanning = false

    var body: some View {
        Form {
            if let saved = model.saved {
                Section {
                    Button {
                        Task { await model.chooseSaved() }
                    } label: {
                        Label(saved.name, systemImage: "server.rack")
                    }
                } header: {
                    Text("Last signed in")
                } footer: {
                    if identityChanged { IdentityChanged() }
                }
            }

            Section {
                ForEach(model.discovered, id: \.info.id) { found in
                    Button {
                        model.choose(found)
                    } label: {
                        Label(found.info.name, systemImage: "server.rack")
                    }
                }
                if model.browser.denied {
                    Text("Local Network access is off. Allow it to find servers on this network.")
                        .foregroundStyle(palette.textMuted)
                    Button(String(localized: "Open Settings")) {
                        if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
                    }
                } else if model.discovering {
                    HStack(spacing: KivoTokens.spacing * 2) {
                        ProgressView()
                        Text("Looking for servers on this network…").foregroundStyle(palette.textMuted)
                    }
                } else if model.discovered.isEmpty {
                    Text("No servers found. Enter the address, or find it on a signed-in device under Settings › Link a device.")
                        .foregroundStyle(palette.textMuted)
                }
            } header: {
                Text("On this network")
            }

            Section {
                TextField(String(localized: "192.168.1.20 or kivo.example.com"), text: $model.address)
                    .accessibilityIdentifier("address")
                    .textContentType(.URL)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.continue)
                    .onSubmit { Task { await model.connect() } }
                Button("Continue") { Task { await model.connect() } }
                    .disabled(model.address.isEmpty || model.busy)
            } header: {
                Text("Enter address")
            }

            if QRScanner.available {
                Section {
                    Button(String(localized: "Scan server QR code")) { scanning = true }
                } footer: {
                    Text("A signed-in device shows it under Settings › Link a device › Show server address.")
                }
            }

            if let message = model.message {
                Section { Text(message).foregroundStyle(palette.danger) }
            }
        }
        .disabled(model.busy)
        .overlay { if model.busy { ProgressView() } }
        // The lockup above the title, at the leading edge, as on the web.
        .toolbar {
            ToolbarItem(placement: .topBarLeading) { KivoLockup() }
                .kivoPlainBackground()
        }
        .sheet(isPresented: $scanning) {
            NavigationStack {
                QRScanner { text in
                    scanning = false
                    Task { await model.scanned(text) }
                }
                .ignoresSafeArea()
                .navigationTitle(String(localized: "Scan server QR code"))
                .toolbarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "Cancel")) { scanning = false }
                    }
                }
            }
        }
    }
}

private struct PasswordForm: View {
    @Bindable var model: SignInModel
    let found: FoundServer
    let identityChanged: Bool
    @Environment(\.kivoPalette) private var palette
    @FocusState private var focus: Field?

    private enum Field { case username, password }

    var body: some View {
        Form {
            if identityChanged {
                Section { IdentityChanged() }
            }
            Section {
                TextField(String(localized: "Username"), text: $model.username)
                    .accessibilityIdentifier("username")
                    .textContentType(.username)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .focused($focus, equals: .username)
                    .submitLabel(.next)
                    .onSubmit { focus = .password }
                SecureField(String(localized: "Password"), text: $model.password)
                    .accessibilityIdentifier("password")
                    .textContentType(.password)
                    .focused($focus, equals: .password)
                    .submitLabel(.go)
                    .onSubmit { Task { await model.signIn(to: found) } }
            } header: {
                Text(found.info.name)
            } footer: {
                Text(found.baseUrl)
            }

            Section {
                Button("Sign in") { Task { await model.signIn(to: found) } }
                    .disabled(model.username.isEmpty || model.password.isEmpty || model.busy)
            }

            if let message = model.message {
                Section { Text(message).foregroundStyle(palette.danger) }
            }

            Section {
                Button(String(localized: "Sign in with another device")) { model.linking = true }
                    .accessibilityIdentifier("sign-in-with-another-device")
            } footer: {
                Text("Approve a code on a device that's signed in.")
            }
        }
        .disabled(model.busy)
        .overlay { if model.busy { ProgressView() } }
        .navigationDestination(isPresented: $model.linking) {
            DeviceCodeView(found: found, name: AppleIdentity.modelName)
        }
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Button(String(localized: "Change server")) { model.back() }
            }
        }
        .onAppear { focus = .username }
    }
}

/// Kivo's logo: the Facet K beside "Kivo", in the tints for the theme's
/// background, the height of the web's sign-in logo.
private struct KivoLockup: View {
    private let height = KivoTokens.spacing * 7
    /// The lockup drawing's width over its height (lockup.svg's viewBox).
    private let aspect: CGFloat = 165.1 / 46

    var body: some View {
        Image("Logo")
            .renderingMode(.original)
            .resizable()
            .aspectRatio(contentMode: .fit)
            .frame(width: height * aspect, height: height)
            .fixedSize()
            // iOS 26 pads a bar item a spacing unit in from the large
            // title's edge: back out, so the K lines up with the title.
            .offset(x: leadingOffset)
            .accessibilityLabel(Text("Kivo"))
    }

    private var leadingOffset: CGFloat {
        if #available(iOS 26, *) { -KivoTokens.spacing } else { 0 }
    }
}

private extension ToolbarItem {
    /// Without the bar's shared glass behind it, where there is one.
    @ToolbarContentBuilder
    func kivoPlainBackground() -> some ToolbarContent {
        if #available(iOS 26, *) {
            sharedBackgroundVisibility(.hidden)
        } else {
            self
        }
    }
}

/// The warning that the last server's session ended as its identity changed.
private struct IdentityChanged: View {
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Text("This server's identity changed. Sign in again if you reinstalled or moved Kivo. If you didn't, don't.")
            .foregroundStyle(palette.danger)
            .accessibilityIdentifier("identityChanged")
    }
}
