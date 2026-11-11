import KivoCore
import KivoUI
import SwiftUI

/// Downloads a season's or a series' episodes: each quality
/// with the episodes' total size, the episodes already here or on their
/// way left as they are.
struct DownloadEpisodesSheet: View {
    let title: String
    /// The episodes to download, read as the sheet opens.
    let episodes: () async throws -> [String]
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    @State private var estimates: [DownloadEstimate]?
    @State private var problem: String?

    var body: some View {
        NavigationStack {
            List {
                if let estimates {
                    let wanted = estimates.filter { $0.playable && server.download(of: $0.workId) == nil }
                    Section {
                        ForEach(qualities(of: wanted), id: \.quality) { option in
                            Button {
                                download(wanted, at: option.quality)
                            } label: {
                                HStack {
                                    Text(DownloadQuality.companion.parseOrNull(value: option.quality).map { server.display.downloadQuality(quality: $0) } ?? option.quality)
                                        .foregroundStyle(palette.text)
                                    Spacer()
                                    if option.quality == server.app.settings?.downloadQuality.value {
                                        Image(systemName: "checkmark").foregroundStyle(palette.accent)
                                    }
                                }
                            }
                            .accessibilityIdentifier("download-\(option.quality)")
                        }
                    } footer: {
                        Text(wanted.count == 1 ? String(localized: "1 episode to download.") : String(localized: "\(wanted.count) episodes to download."))
                    }
                } else {
                    ProgressView().frame(maxWidth: .infinity)
                }
            }
            .navigationTitle(title)
            .toolbarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "Cancel")) { dismiss() }
                }
            }
        }
        .presentationDetents([.medium])
        .task {
            do {
                let ids = try await episodes()
                estimates = try await server.session.downloads?.estimates(choices: ids.map { PlayChoice(workId: $0) }) ?? []
            } catch {
                problem = error.downloadSentence
            }
        }
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil; dismiss() } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
    }

    /// The qualities on offer, as the first episode lists them.
    private func qualities(of estimates: [DownloadEstimate]) -> [DownloadOption] {
        estimates.first?.options ?? []
    }

    private func download(_ estimates: [DownloadEstimate], at quality: String) {
        Haptics.success()
        Task {
            do {
                for estimate in estimates {
                    _ = try await server.session.downloads?.download(
                        choice: PlayChoice(workId: estimate.workId),
                        quality: DownloadQuality.allCases.first { $0.value == quality }
                    )
                }
                dismiss()
            } catch {
                problem = error.downloadSentence
            }
        }
    }
}
