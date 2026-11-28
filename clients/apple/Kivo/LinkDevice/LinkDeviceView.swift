import KivoCore
import KivoUI
import SwiftUI

/// Link a device: scan the QR code another device shows, or enter its code,
/// then "Allow Living Room to sign in as Anna?" with what the server saw of
/// it, and Allow or Don't allow (LinkDeviceModel). A device on another
/// server, or one asking from outside the home network, is said so. Show
/// server address shows the server's address as text and as a QR code, for
/// a device that can't find the server by itself.
struct LinkDeviceView: View {
    /// A code or link to look up as the view opens, such as a `kivo://link` link's.
    var initial: String?

    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    @State private var model: LinkDeviceModel?
    @State private var scanning = QRScanner.available || QRScanner.testScan != nil
    @State private var typed = ""

    var body: some View {
        ScrollView {
            VStack(spacing: KivoTokens.spacing * 6) {
                if let model {
                    content(model)
                    if let message = model.message {
                        Text(message)
                            .foregroundStyle(palette.danger)
                            .multilineTextAlignment(.center)
                    }
                    if model.busy { ProgressView() }
                }
            }
            .padding(KivoTokens.spacing * 6)
            .frame(maxWidth: KivoTokens.Container.form)
            .frame(maxWidth: .infinity)
        }
        .background(palette.bg)
        .navigationTitle(String(localized: "Link a device"))
        .toolbarTitleDisplayMode(.inline)
        .task {
            let model = model ?? LinkDeviceModel(core: app.core, session: server.session)
            self.model = model
            if let initial {
                await look(up: initial, in: model)
            } else if scanning, let scan = QRScanner.testScan {
                await look(up: scan, in: model)
            }
        }
    }

    @ViewBuilder private func content(_ model: LinkDeviceModel) -> some View {
        switch model.step {
        case .enter where scanning:
            QRScanner { scanned in Task { await look(up: scanned, in: model) } }
                .aspectRatio(1, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.lg))
            Text("Scan the QR code on the other device.")
                .foregroundStyle(palette.textMuted)
            Button(String(localized: "Enter code instead")) { scanning = false }
            serverAddress
        case .enter:
            TextField(String(localized: "Code"), text: $typed)
                .accessibilityIdentifier("link-code-field")
                .textInputAutocapitalization(.characters)
                .autocorrectionDisabled()
                .font(.title.monospaced())
                .multilineTextAlignment(.center)
                .textFieldStyle(.roundedBorder)
                .onSubmit { Task { await look(up: typed, in: model) } }
            Text("Codes are six letters and digits.")
                .font(.footnote)
                .foregroundStyle(palette.textMuted)
            Button(String(localized: "Continue")) { Task { await look(up: typed, in: model) } }
                .buttonStyle(.borderedProminent)
                .disabled(typed.isEmpty || model.busy)
            if QRScanner.available {
                Button(String(localized: "Scan the QR code instead")) { scanning = true }
            }
            serverAddress
        case let .otherServer(name):
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 44))
                .foregroundStyle(palette.textMuted)
            Text(name.map { String(localized: "This device is signing in to \($0).") } ?? String(localized: "This device is signing in to another server."))
                .kivoStyle(KivoTokens.TextSize.xl, weight: KivoTokens.Weight.semibold)
                .multilineTextAlignment(.center)
            Text("You're signed in to \(server.serverName). Sign in to that server here to link it.")
                .multilineTextAlignment(.center)
                .foregroundStyle(palette.textMuted)
            Button(String(localized: "Done")) { dismiss() }
                .buttonStyle(.borderedProminent)
        case let .confirm(code, request):
            Text("Allow \(request.name) to sign in as \(server.user?.username ?? "")?")
                .kivoStyle(KivoTokens.TextSize.xl, weight: KivoTokens.Weight.semibold)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("link-question")
            Text(LinkDeviceModel.context(of: request))
                .foregroundStyle(palette.textMuted)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("link-context")
            if !request.nearby {
                Label(
                    String(localized: "This device isn't on your home network. Allow it only if it's yours and you just started signing in on it."),
                    systemImage: "exclamationmark.triangle.fill"
                )
                .foregroundStyle(palette.danger)
                .accessibilityIdentifier("link-warning")
            }
            HStack(spacing: KivoTokens.spacing * 3) {
                Button(String(localized: "Don't allow")) { Task { await model.answer(code: code, request: request, allow: false) } }
                    .buttonStyle(.bordered)
                Button(String(localized: "Allow")) { Task { await model.answer(code: code, request: request, allow: true) } }
                    .buttonStyle(.borderedProminent)
            }
            .disabled(model.busy)
        case let .done(name, allowed):
            Image(systemName: allowed ? "checkmark.circle.fill" : "xmark.circle.fill")
                .font(.system(size: 56))
                .foregroundStyle(allowed ? palette.success : palette.textMuted)
            Text(allowed ? String(localized: "\(name) is signed in") : String(localized: "\(name) wasn't signed in"))
                .kivoStyle(KivoTokens.TextSize.xl, weight: KivoTokens.Weight.semibold)
                .multilineTextAlignment(.center)
            Button(String(localized: "Done")) { dismiss() }
                .buttonStyle(.borderedProminent)
        }
    }

    private var serverAddress: some View {
        NavigationLink(String(localized: "Show server address")) { ServerAddressView() }
            .padding(.top, KivoTokens.spacing * 4)
    }

    /// The device waiting on the code in what was scanned or typed; the scanner gives way once it's found.
    private func look(up text: String, in model: LinkDeviceModel) async {
        if await model.look(up: text) { scanning = false }
    }
}
