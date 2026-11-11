import KivoCore
import KivoUI
import SwiftUI

/// Link a device: scan the QR code another device shows, or enter its code,
/// then "Allow Living Room to sign in as Anna?" with what the server saw of
/// it, and Allow or Don't allow. A device on another server, or one asking
/// from outside the home network, is said so. Show server address shows
/// the server's address as text and as a QR code, for a device that can't
/// find the server by itself.
struct LinkDeviceView: View {
    private enum Step {
        case scan
        case enter
        case otherServer(name: String?)
        case confirm(code: String, request: PendingDevice)
        case done(name: String, allowed: Bool)
    }

    /// A code or link to look up as the view opens, such as a `kivo://link` link's.
    var initial: String?

    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    @State private var step: Step = QRScanner.available || QRScanner.testScan != nil ? .scan : .enter
    @State private var typed = ""
    @State private var busy = false
    @State private var message: String?

    var body: some View {
        ScrollView {
            VStack(spacing: KivoTokens.spacing * 6) {
                content
                if let message {
                    Text(message)
                        .foregroundStyle(palette.danger)
                        .multilineTextAlignment(.center)
                }
                if busy { ProgressView() }
            }
            .padding(KivoTokens.spacing * 6)
            .frame(maxWidth: KivoTokens.Container.form)
            .frame(maxWidth: .infinity)
        }
        .background(palette.bg)
        .navigationTitle(String(localized: "Link a device"))
        .toolbarTitleDisplayMode(.inline)
        .task {
            if let initial {
                await look(up: initial)
            } else if case .scan = step, let scan = QRScanner.testScan {
                await look(up: scan)
            }
        }
    }

    @ViewBuilder private var content: some View {
        switch step {
        case .scan:
            QRScanner { scanned in Task { await look(up: scanned) } }
                .aspectRatio(1, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.lg))
            Text("Scan the QR code on the other device.")
                .foregroundStyle(palette.textMuted)
            Button(String(localized: "Enter code instead")) { step = .enter }
            serverAddress
        case .enter:
            TextField(String(localized: "Code"), text: $typed)
                .accessibilityIdentifier("link-code-field")
                .textInputAutocapitalization(.characters)
                .autocorrectionDisabled()
                .font(.title.monospaced())
                .multilineTextAlignment(.center)
                .textFieldStyle(.roundedBorder)
                .onSubmit { Task { await look(up: typed) } }
            Text("Codes are six letters and digits.")
                .font(.footnote)
                .foregroundStyle(palette.textMuted)
            Button(String(localized: "Continue")) { Task { await look(up: typed) } }
                .buttonStyle(.borderedProminent)
                .disabled(typed.isEmpty || busy)
            if QRScanner.available {
                Button(String(localized: "Scan the QR code instead")) { step = .scan }
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
            Text(Self.context(of: request))
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
                Button(String(localized: "Don't allow")) { Task { await answer(code: code, request: request, allow: false) } }
                    .buttonStyle(.bordered)
                Button(String(localized: "Allow")) { Task { await answer(code: code, request: request, allow: true) } }
                    .buttonStyle(.borderedProminent)
            }
            .disabled(busy)
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

    /// The device waiting on the code in what was scanned or typed.
    private func look(up text: String) async {
        guard !busy else { return }
        guard let link = LinkCode.companion.parseOrNull(text: text) else {
            message = String(localized: "That isn't a code. Codes are six letters and digits.")
            return
        }
        if let other = link.server, other != server.session.savedServer.value.id {
            var name: String?
            for await servers in app.core.servers {
                name = servers.first { $0.id == other }?.name
                break
            }
            message = nil
            step = .otherServer(name: name)
            return
        }
        busy = true
        defer { busy = false }
        do {
            let request = try await server.session.link.pendingDevice(code: link.code)
            message = nil
            step = .confirm(code: link.code, request: request)
        } catch {
            message = codeFailure(error)
        }
    }

    private func answer(code: String, request: PendingDevice, allow: Bool) async {
        busy = true
        defer { busy = false }
        do {
            if allow {
                try await server.session.link.approve(code: code)
            } else {
                try await server.session.link.deny(code: code)
            }
            message = nil
            step = .done(name: request.name, allowed: allow)
        } catch {
            message = codeFailure(error)
        }
    }

    private func codeFailure(_ error: Error) -> String {
        error.problemStatus == 404
            ? String(localized: "That code didn't work. Check the other device for a new one.")
            : error.problemSentence
    }

    /// What the server saw of the asking device: "Android TV · On your home network · Just now".
    static func context(of request: PendingDevice) -> String {
        let place = request.nearby ? String(localized: "On your home network") : String(localized: "Not on your home network")
        let started = Date(timeIntervalSince1970: TimeInterval(request.startedAt.toEpochMilliseconds()) / 1000)
        let when = Date.now.timeIntervalSince(started) < 60
            ? String(localized: "Just now")
            : started.formatted(.relative(presentation: .named))
        return [Platform.name(request.platform), place, when].compactMap { $0 }.joined(separator: " · ")
    }
}

/// The platforms a client says it runs on, for people.
enum Platform {
    static func name(_ platform: ClientPlatform?) -> String? {
        switch platform {
        case .androidTv: String(localized: "Android TV")
        case .appleTv: String(localized: "Apple TV")
        case .iphone: String(localized: "iPhone")
        case .ipad: String(localized: "iPad")
        case .androidPhone: String(localized: "Android phone")
        case .browser: String(localized: "Browser")
        case .cast, .tizen, .unknown, nil: nil
        }
    }
}
