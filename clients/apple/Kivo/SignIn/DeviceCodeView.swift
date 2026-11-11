import KivoCore
import KivoUI
import SwiftUI

/// Sign in with another device: the code and its QR code, for a device
/// that's signed in to approve. An expired code gives way to a new one;
/// a declined one says so and shows a new one; once it's approved, the
/// app is signed in.
struct DeviceCodeView: View {
    let found: FoundServer
    let name: String

    @Environment(AppModel.self) private var app
    @Environment(\.kivoPalette) private var palette
    @State private var waiting: LinkProgressWaiting?
    @State private var message: String?
    @State private var failed = false
    /// Counts up to start a new code.
    @State private var attempt = 0

    var body: some View {
        ScrollView {
            VStack(spacing: KivoTokens.spacing * 5) {
                if let waiting {
                    Text("On a device that's signed in, open Settings › Link a device and scan this code, or enter it.")
                        .multilineTextAlignment(.center)
                        .foregroundStyle(palette.textMuted)
                    QRCode(text: waiting.linkUrl)
                        .frame(maxWidth: 240)
                    Text(Self.grouped(waiting.code))
                        .font(.system(.largeTitle, design: .monospaced).weight(.semibold))
                        .accessibilityIdentifier("link-code")
                        .accessibilityLabel(waiting.code)
                } else if !failed {
                    ProgressView()
                }
                if let message {
                    Text(message)
                        .multilineTextAlignment(.center)
                        .foregroundStyle(palette.danger)
                }
                if failed {
                    Button(String(localized: "Try again")) { attempt += 1 }
                        .buttonStyle(.borderedProminent)
                }
            }
            .padding(KivoTokens.spacing * 6)
            .frame(maxWidth: .infinity)
        }
        .background(palette.bg)
        .navigationTitle(String(localized: "Sign in with another device"))
        .toolbarTitleDisplayMode(.inline)
        .task(id: attempt) { await link() }
    }

    /// Follows codes until one is approved, or the link fails.
    private func link() async {
        failed = false
        while !Task.isCancelled {
            for await progress in app.core.signInByLink(found: found, name: name) {
                switch onEnum(of: progress) {
                case let .waiting(next):
                    waiting = next
                case let .signedIn(signedIn):
                    app.core.signedIn(session: signedIn.session)
                    return
                case .denied:
                    message = String(localized: "Sign-in was declined.")
                case .expired:
                    message = nil
                case let .failed(failure):
                    waiting = nil
                    message = failure.problem.sentence
                    failed = true
                    return
                }
            }
        }
    }

    /// A code in two groups of three, "K7Q-2M9", to read across a room.
    static func grouped(_ code: String) -> String {
        code.count == 6 ? "\(code.prefix(3))-\(code.suffix(3))" : code
    }
}
