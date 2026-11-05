import KivoCore
import KivoUI
import SwiftUI

/// Under a detail page's actions, once a download is asked for: a small
/// "Downloaded" when it's ready, else where it is, "Downloading · 42%".
struct DownloadBadge: View {
    let workId: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        if let item = server.download(of: workId) {
            Label {
                Text(DownloadWords.badge(item))
                    .monospacedDigit()
            } icon: {
                Image(systemName: item.status is DownloadStatusReady ? "checkmark.circle.fill" : "arrow.down.circle")
            }
            .font(.footnote.weight(.medium))
            .foregroundStyle(item.status is DownloadStatusFailed ? palette.danger : palette.textMuted)
            .padding(.horizontal, KivoTokens.spacing * 2.5)
            .padding(.vertical, KivoTokens.spacing)
            .background(palette.surfaceRaised, in: Capsule())
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("download-badge")
        }
    }
}
