import KivoCore
import KivoUI
import SwiftUI

/// The server's address as text and as a QR code: its public URL when it
/// has one, else its first local URL. A new phone scans it to find the
/// server; on a TV it's read off and typed. It's an address only.
struct ServerAddressView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var address: String?
    @State private var failure: String?

    var body: some View {
        VStack(spacing: KivoTokens.spacing * 5) {
            if let address {
                QRCode(text: address)
                    .frame(maxWidth: 240)
                Text(address)
                    .font(.title3.monospaced())
                    .textSelection(.enabled)
                    .multilineTextAlignment(.center)
                    .accessibilityIdentifier("server-address")
                Text("On a new phone, scan this under Sign in › Scan server QR code.")
                    .multilineTextAlignment(.center)
                    .foregroundStyle(palette.textMuted)
            } else if let failure {
                FailureRow(message: failure) { Task { await load() } }
            } else {
                ProgressView()
            }
            Spacer()
        }
        .padding(KivoTokens.spacing * 6)
        .frame(maxWidth: .infinity)
        .background(palette.bg)
        .navigationTitle(String(localized: "Server address"))
        .toolbarTitleDisplayMode(.inline)
        .task { await load() }
    }

    private func load() async {
        do {
            let info = try await server.session.serverInfo()
            address = info.publicUrl ?? info.localUrls?.first ?? server.session.baseUrl
            failure = nil
        } catch {
            failure = error.problemSentence
        }
    }
}
