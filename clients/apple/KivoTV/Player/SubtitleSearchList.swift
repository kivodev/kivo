import KivoCore
import KivoUI
import SwiftUI

/// Search subtitles, over the player or a title's page: the results of
/// [search] in the server's order, each its language and flags over the
/// release it was made for. Choosing one [download]s it and closes, with
/// the subtitle chosen; a failure says why.
struct SubtitleSearchList: View {
    let display: Display
    let search: () async throws -> [SubtitleResult]
    let download: (SubtitleResult) async throws -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var results: [SubtitleResult]?
    @State private var failure: String?
    @State private var downloading: String?
    @State private var downloadFailure: String?

    var body: some View {
        NavigationStack {
            Group {
                if let failure {
                    ContentUnavailableView(failure, systemImage: "captions.bubble")
                } else if let results, results.isEmpty {
                    ContentUnavailableView(String(localized: "No subtitles found"), systemImage: "captions.bubble")
                } else if let results {
                    List(results, id: \.id) { result in
                        row(result)
                    }
                } else {
                    ProgressView()
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            }
            .navigationTitle(String(localized: "Search Subtitles"))
        }
        .task {
            do {
                results = try await search()
            } catch {
                failure = error.subtitleSearchSentence
            }
        }
        .alert(downloadFailure ?? "", isPresented: Binding(get: { downloadFailure != nil }, set: { if !$0 { downloadFailure = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
    }

    private func row(_ result: SubtitleResult) -> some View {
        let name = display.subtitleResultName(result: result, exactMatch: String(localized: "Exact match"))
        return Button {
            choose(result)
        } label: {
            HStack {
                VStack(alignment: .leading, spacing: KivoTokens.spacing) {
                    Text(name.name)
                    if let note = name.note {
                        Text(note)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                    }
                }
                Spacer()
                if downloading == result.id {
                    ProgressView()
                }
            }
        }
        .disabled(downloading != nil)
    }

    private func choose(_ result: SubtitleResult) {
        guard downloading == nil else { return }
        downloading = result.id
        Task {
            defer { downloading = nil }
            do {
                try await download(result)
                dismiss()
            } catch {
                downloadFailure = error.subtitleSearchSentence
            }
        }
    }
}
