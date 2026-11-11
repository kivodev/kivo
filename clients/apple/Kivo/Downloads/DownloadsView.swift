import KivoCore
import KivoUI
import SwiftUI

/// Downloads: Books, Movies and Shows, a show opening to its
/// episodes by season, each with its artwork, title and one line of
/// state, newest first, and while it's under way a ring at its end.
/// A row swipes to delete, a show all its episodes once confirmed; Edit
/// selects rows to delete together. While downloads convert or come, Keep
/// screen on keeps the screen on as the page shows.
/// Its foot gives the space they take and what's free. A ready download
/// plays here from its files, with or without the server.
struct DownloadsView: View {
    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server
    @State private var editMode: EditMode = .inactive
    /// Selected rows: a download's ID, or a show's series ID for all its episodes.
    @State private var selection: Set<String> = []

    var body: some View {
        @Bindable var app = app
        let items = server.downloads
        let coming = items.contains {
            $0.status is DownloadStatusQueued || $0.status is DownloadStatusConverting || $0.status is DownloadStatusDownloading
        }
        let books = items.filter { $0.kind == .book }
        let movies = items.filter { $0.kind != .book && $0.seriesId == nil }
        let shows = Dictionary(grouping: items.filter { $0.seriesId != nil }) { $0.seriesId ?? "" }
            .sorted { ($0.value.map(\.createdAtMs).max() ?? 0) > ($1.value.map(\.createdAtMs).max() ?? 0) }
        Group {
            if items.isEmpty {
                ContentUnavailableView {
                    Label(String(localized: "Downloads"), systemImage: "arrow.down.circle")
                } description: {
                    Text("Downloaded books and videos appear here for offline playback.")
                } actions: {
                    if server.deletedDownloads > 0 { DeletedNotice(count: server.deletedDownloads) }
                }
            } else {
                List(selection: editMode.isEditing ? $selection : nil) {
                    if server.deletedDownloads > 0 {
                        DeletedNotice(count: server.deletedDownloads)
                            .listRowBackground(Color.clear)
                            .selectionDisabled()
                    }
                    if coming {
                        Section {
                            Toggle(String(localized: "Keep screen on"), isOn: $app.keepScreenOn)
                                .selectionDisabled()
                        }
                    }
                    if !books.isEmpty {
                        Section(String(localized: "Books")) { rows(books) }
                    }
                    if !movies.isEmpty {
                        Section(String(localized: "Movies")) { rows(movies) }
                    }
                    if !shows.isEmpty {
                        Section(String(localized: "Shows")) {
                            ForEach(shows, id: \.key) { show in
                                ShowItem(seriesId: show.key, episodes: show.value)
                            }
                        }
                    }
                    Section {} footer: { StorageLine() }
                }
                .listStyle(.insetGrouped)
                .readableRows()
                .scrollContentBackground(.hidden)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .downloadEditing(
            title: String(localized: "Downloads"), large: true, rows: (books + movies).map(\.id) + shows.map(\.key), items: items,
            editMode: $editMode, selection: $selection
        )
        // As the page shows, and as downloads or the setting change.
        .task(id: coming && app.keepScreenOn) {
            UIApplication.shared.isIdleTimerDisabled = coming && app.keepScreenOn
        }
        .onDisappear {
            UIApplication.shared.isIdleTimerDisabled = false
            if server.deletedDownloads > 0 { server.session.downloads?.deletionsSeen() }
        }
    }

    private func rows(_ items: [DownloadItem]) -> some View {
        ForEach(items, id: \.id) { item in
            DownloadRow(item: item)
        }
    }
}

/// A show's downloaded episodes, by season and number, with Edit to
/// delete several at once.
private struct ShowDownloadsView: View {
    let seriesId: String
    let title: String
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    @State private var editMode: EditMode = .inactive
    @State private var selection: Set<String> = []

    var body: some View {
        let episodes = server.downloads
            .filter { $0.seriesId == seriesId }
            .sorted { ($0.season?.intValue ?? 0, $0.episode?.intValue ?? 0) < ($1.season?.intValue ?? 0, $1.episode?.intValue ?? 0) }
        let seasons = Dictionary(grouping: episodes) { $0.season?.intValue ?? 0 }.sorted { $0.key < $1.key }
        List(selection: editMode.isEditing ? $selection : nil) {
            ForEach(seasons, id: \.key) { season in
                Section(season.key == 0 ? String(localized: "Specials") : String(localized: "Season \(season.key)")) {
                    ForEach(season.value, id: \.id) { DownloadRow(item: $0, numbered: true) }
                }
            }
            Section {} footer: { StorageLine() }
        }
        .listStyle(.insetGrouped)
        .readableRows()
        .scrollContentBackground(.hidden)
        .downloadEditing(title: title, large: false, rows: episodes.map(\.id), items: episodes, editMode: $editMode, selection: $selection)
        .onChange(of: episodes.isEmpty) {
            if episodes.isEmpty { dismiss() }
        }
    }
}

private extension View {
    /// Edit for a list of downloads: [title], or "3 Selected" while rows are selected, and a bottom
    /// bar with Select All or Deselect All and Delete, which asks once for every row selected.
    func downloadEditing(
        title: String, large: Bool, rows: [String], items: [DownloadItem],
        editMode: Binding<EditMode>, selection: Binding<Set<String>>
    ) -> some View {
        modifier(DownloadEditing(title: title, large: large, rows: rows, items: items, editMode: editMode, selection: selection))
    }
}

/// [rows] are the selection IDs: a download's ID, or a show's series ID for all its episodes.
private struct DownloadEditing: ViewModifier {
    let title: String
    let large: Bool
    let rows: [String]
    let items: [DownloadItem]
    @Binding var editMode: EditMode
    @Binding var selection: Set<String>
    @Environment(\.kivoPalette) private var palette
    @State private var deleting: DownloadDeletion?

    func body(content: Content) -> some View {
        let editing = editMode.isEditing
        let selected = selection.intersection(rows)
        content
            .navigationTitle(editing && !selected.isEmpty ? String(localized: "\(selected.count) Selected") : title)
            .toolbarTitleDisplayMode(large && !editing ? .large : .inline)
            .environment(\.editMode, $editMode)
            .toolbar {
                if !rows.isEmpty {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button(editing ? String(localized: "Done") : String(localized: "Edit")) {
                            withAnimation {
                                editMode = editing ? .inactive : .active
                                selection = []
                            }
                        }
                        .fontWeight(editing ? .semibold : .regular)
                        .accessibilityIdentifier("downloads-edit")
                    }
                }
                if editing {
                    ToolbarItemGroup(placement: .bottomBar) {
                        let all = !rows.isEmpty && selected.count == rows.count
                        Button(all ? String(localized: "Deselect All") : String(localized: "Select All")) {
                            selection = all ? [] : Set(rows)
                        }
                        .accessibilityIdentifier("downloads-select-all")
                        Spacer()
                        Button(selected.isEmpty ? String(localized: "Delete") : String(localized: "Delete (\(selected.count))"), role: .destructive) {
                            deleting = DownloadDeletion(
                                rows: selected.count,
                                items: items.filter { selected.contains($0.id) || selected.contains($0.seriesId ?? "") }
                            )
                        }
                        .tint(palette.danger)
                        .disabled(selected.isEmpty)
                        .accessibilityIdentifier("downloads-delete-selected")
                    }
                }
            }
            .toolbar(editing ? .hidden : .automatic, for: .tabBar)
            .downloadDeletion($deleting) {
                withAnimation {
                    editMode = .inactive
                    selection = []
                }
            }
            .onChange(of: rows.isEmpty) {
                if rows.isEmpty { editMode = .inactive }
            }
    }
}

/// A show in Downloads: it opens to its episodes, or while editing is
/// selected, and swipes to delete them all once confirmed. Its row draws
/// its own end, so the list's chevron is hidden behind it.
private struct ShowItem: View {
    let seriesId: String
    let episodes: [DownloadItem]
    @Environment(\.editMode) private var editMode
    @State private var deleting: DownloadDeletion?

    var body: some View {
        ShowRow(episodes: episodes, editing: editMode?.wrappedValue.isEditing == true)
            .background {
                if editMode?.wrappedValue.isEditing != true {
                    NavigationLink {
                        ShowDownloadsView(seriesId: seriesId, title: episodes.first?.seriesTitle ?? "")
                            .kivoPageBackground()
                    } label: {
                        EmptyView()
                    }
                    .opacity(0)
                }
            }
            .swipeActions(edge: .trailing) {
                Button {
                    deleting = DownloadDeletion(episodes: episodes)
                } label: {
                    Label(String(localized: "Delete"), systemImage: "trash")
                }
                .tint(.red)
            }
            .downloadDeletion($deleting) {}
    }
}

/// Downloads about to be deleted together, and how the confirmation asks:
/// "Delete 6 episodes (2.1 GB)?".
private struct DownloadDeletion {
    let ids: [String]
    /// "Delete 6 episodes (2.1 GB)", or "Delete 2 items (1.4 GB)".
    let action: String

    var question: String { "\(action)?" }

    /// A show's episodes.
    init(episodes: [DownloadItem]) {
        ids = episodes.map(\.id)
        let size = Display.app.size(bytes: episodes.reduce(Int64(0)) { $0 + $1.sizeBytes })
        action = episodes.count == 1
            ? String(localized: "Delete 1 episode (\(size))")
            : String(localized: "Delete \(episodes.count) episodes (\(size))")
    }

    /// Selected rows, a show among them standing for all its episodes.
    init(rows: Int, items: [DownloadItem]) {
        ids = items.map(\.id)
        let size = Display.app.size(bytes: items.reduce(Int64(0)) { $0 + $1.sizeBytes })
        action = rows == 1
            ? String(localized: "Delete 1 item (\(size))")
            : String(localized: "Delete \(rows) items (\(size))")
    }
}

private extension View {
    /// Asks before deleting [deletion]'s downloads, then deletes them, those under way cancelled.
    func downloadDeletion(_ deletion: Binding<DownloadDeletion?>, deleted: @escaping () -> Void) -> some View {
        modifier(DownloadDeletionDialog(deletion: deletion, deleted: deleted))
    }
}

private struct DownloadDeletionDialog: ViewModifier {
    @Binding var deletion: DownloadDeletion?
    let deleted: () -> Void
    @Environment(ServerModel.self) private var server

    func body(content: Content) -> some View {
        content.confirmationDialog(
            deletion?.question ?? "",
            isPresented: Binding(get: { deletion != nil }, set: { if !$0 { deletion = nil } }),
            titleVisibility: .visible,
            presenting: deletion
        ) { deletion in
            Button(String(localized: "Delete"), role: .destructive) {
                deleted()
                // Downloads that can't be deleted stay in the list.
                Task { try? await server.session.downloads?.delete(ids: deletion.ids) }
            }
            .accessibilityIdentifier("download-delete-confirm")
        }
    }
}

/// A show in the list: its series poster, its name, and how many episodes
/// are here and their size, or while any is coming one state for the
/// whole show and how much of it is done. At its end, a ring
/// while any is coming, a red warning when one failed, else a chevron;
/// nothing while editing.
private struct ShowRow: View {
    let episodes: [DownloadItem]
    let editing: Bool
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        let title = episodes.first?.seriesTitle ?? ""
        HStack(spacing: KivoTokens.spacing * 2) {
            HStack(spacing: KivoTokens.spacing * 3) {
                LocalArtwork(path: episodes.lazy.compactMap(\.posterPath).first ?? episodes.first?.artworkPath, title: title)
                    .frame(width: 56, height: 84)
                DownloadText(
                    title: title, line: DownloadWords.line(show: episodes, display: server.display),
                    failed: episodes.allSatisfy { $0.status is DownloadStatusFailed }
                )
            }
            if !editing {
                ControlSlot {
                    if failed {
                        Image(systemName: "exclamationmark.circle")
                            .font(.title3)
                            .foregroundStyle(palette.danger)
                    } else if let progress = DownloadWords.progress(show: episodes) {
                        DownloadRowRing(progress: progress)
                    } else {
                        Image(systemName: "chevron.right")
                            .font(.footnote.weight(.semibold))
                            .foregroundStyle(palette.textMuted.opacity(0.6))
                            .frame(maxWidth: .infinity, alignment: .trailing)
                    }
                }
            }
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityValue(state)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("download-show")
    }

    private var failed: Bool { episodes.contains { $0.status is DownloadStatusFailed } }

    private var state: String {
        if failed { return String(localized: "Download failed") }
        return episodes.allSatisfy { $0.status is DownloadStatusReady } ? String(localized: "Downloaded") : String(localized: "Downloading")
    }
}

/// A row's title, wrapping rather than cut short, and its line.
private struct DownloadText: View {
    let title: String
    let line: String
    let failed: Bool
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title)
                .font(.body.weight(.semibold))
                .foregroundStyle(palette.text)
            Text(line)
                .font(.subheadline)
                .monospacedDigit()
                .foregroundStyle(failed ? palette.danger : palette.textMuted)
                .accessibilityIdentifier("download-state")
        }
        .fixedSize(horizontal: false, vertical: true)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// The end of every row, one size, so rings, buttons and chevrons line up
/// down the list.
private struct ControlSlot<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        content.frame(width: 36, height: 36)
    }
}

/// A download's progress as a ring filling from the top, with [symbol] in
/// its middle when it's a button: the accent while the server prepares it
/// or it comes here, turning while the server prepares it with no percent
/// yet, and grey while it's in line, waits or is paused.
private struct DownloadRowRing: View {
    let progress: DownloadProgress
    var symbol: String?
    @Environment(\.kivoPalette) private var palette
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        let tint = progress.active ? palette.accent : palette.textMuted
        ZStack {
            Circle()
                .stroke(palette.textMuted.opacity(0.3), lineWidth: 2.5)
            if progress.active, progress.fraction == nil {
                TimelineView(.animation(paused: reduceMotion)) { context in
                    Circle()
                        .trim(from: 0, to: 0.25)
                        .stroke(tint, style: StrokeStyle(lineWidth: 2.5, lineCap: .round))
                        .rotationEffect(.degrees(context.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 1) * 360))
                }
            } else if let fraction = progress.fraction, fraction > 0 {
                Circle()
                    .trim(from: 0, to: min(fraction, 1))
                    .stroke(tint, style: StrokeStyle(lineWidth: 2.5, lineCap: .round))
                    .rotationEffect(.degrees(-90))
            }
            if let symbol {
                Image(systemName: symbol)
                    .font(.system(size: 11, weight: .bold))
                    .foregroundStyle(palette.text)
            }
        }
        .frame(width: 30, height: 30)
        .animation(.default, value: progress.fraction)
    }
}

/// One download: its artwork, its title, one line of state, and at its end
/// a ring while it's under way, Pause in it while it comes or waits and
/// Resume while it's paused, a round Retry when it failed, and nothing once
/// it's here. On a show's page ([numbered]) an episode's number leads its
/// title. Tapping a ready one plays it from its files. While editing,
/// tapping selects it, and the end hides.
struct DownloadRow: View {
    let item: DownloadItem
    var numbered = false
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.editMode) private var editMode
    @State private var problem: String?

    var body: some View {
        HStack(spacing: KivoTokens.spacing * 2) {
            if editMode?.wrappedValue.isEditing == true {
                content
            } else {
                Button(action: play) { content }
                    .buttonStyle(.plain)
                    .accessibilityValue(state)
                    .accessibilityIdentifier("download")
                control
            }
        }
        .swipeActions(edge: .trailing) {
            Button(role: .destructive) {
                // A download that can't be deleted stays in the list.
                Task { try? await server.session.downloads?.delete(id: item.id) }
            } label: {
                Label(String(localized: "Delete"), systemImage: "trash")
            }
        }
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
    }

    private var content: some View {
        HStack(spacing: KivoTokens.spacing * 3) {
            LocalArtwork(path: item.artworkPath, title: item.title)
                .frame(width: item.kind == .episode ? 96 : 56, height: item.kind == .episode ? 54 : (item.kind == .book ? 56 : 84))
            DownloadText(
                title: DownloadWords.title(item, numbered: numbered), line: DownloadWords.line(item, display: server.display),
                failed: item.status is DownloadStatusFailed
            )
        }
        .contentShape(Rectangle())
    }

    /// What the line doesn't say: that it's here, or that it failed.
    private var state: String {
        switch onEnum(of: item.status) {
        case .ready: DownloadWords.badge(item)
        case .failed: String(localized: "Download failed")
        default: ""
        }
    }

    @ViewBuilder private var control: some View {
        if item.status is DownloadStatusReady {
            EmptyView()
        } else {
            ControlSlot {
                switch onEnum(of: item.status) {
                case .downloading:
                    pause
                case let .waiting(waiting) where waiting.reason != .turn:
                    pause
                case .paused:
                    ring("arrow.down", label: String(localized: "Resume download"), id: "download-resume") {
                        try await server.session.downloads?.resume(id: item.id)
                    }
                case .failed:
                    Button {
                        act { try await server.session.downloads?.retry(id: item.id) }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 13, weight: .bold))
                            .foregroundStyle(palette.text)
                            .frame(width: 30, height: 30)
                            .background(palette.textMuted.opacity(0.3), in: Circle())
                            .frame(width: 44, height: 44)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(Text("Retry"))
                    .accessibilityIdentifier("download-retry")
                default:
                    if let progress = DownloadWords.progress(item) {
                        DownloadRowRing(progress: progress)
                            .accessibilityHidden(true)
                    }
                }
            }
        }
    }

    private var pause: some View {
        ring("pause.fill", label: String(localized: "Pause download"), id: "download-pause") {
            try await server.session.downloads?.pause(id: item.id)
        }
    }

    /// The row's ring as a button, with [symbol] in its middle.
    private func ring(_ symbol: String, label: String, id: String, action: @escaping () async throws -> Void) -> some View {
        Button {
            act(action)
        } label: {
            if let progress = DownloadWords.progress(item) {
                DownloadRowRing(progress: progress, symbol: symbol)
                    .frame(width: 44, height: 44)
                    .contentShape(Rectangle())
            }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(label))
        .accessibilityIdentifier(id)
    }

    private func play() {
        guard item.status is DownloadStatusReady else { return }
        if item.kind == .book {
            server.listen(item.workId)
        } else {
            server.player.play(PlayChoice(workId: item.workId), downloadId: item.id)
        }
    }

    private func act(_ action: @escaping () async throws -> Void) {
        Task {
            do {
                try await action()
            } catch {
                problem = error.downloadSentence
            }
        }
    }
}

/// "2 downloads were removed because they're no longer available to you."
private struct DeletedNotice: View {
    let count: Int32
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Text(count == 1
            ? String(localized: "1 download removed: no longer available.")
            : String(localized: "\(count) downloads removed: no longer available."))
            .font(.footnote)
            .foregroundStyle(palette.textMuted)
    }
}

/// "3.2 GB used · 41 GB free".
private struct StorageLine: View {
    @Environment(ServerModel.self) private var server
    @State private var storage: DownloadStorage?

    var body: some View {
        Text(storage.map { String(localized: "\(Display.app.size(bytes: $0.usedBytes)) used · \(Display.app.size(bytes: $0.freeBytes)) free") } ?? " ")
            .frame(maxWidth: .infinity)
            .accessibilityIdentifier("download-storage")
            .task(id: server.downloads.map { String(describing: $0.status) }) {
                storage = try? await server.session.downloads?.storage()
            }
    }
}

/// Artwork from a download's file, or the title on a plain tile, filling
/// and clipped to the frame it's given.
struct LocalArtwork: View {
    let path: String?
    let title: String
    @Environment(\.kivoPalette) private var palette
    @State private var image: UIImage?

    var body: some View {
        palette.surfaceRaised
            .overlay {
                if let image {
                    Image(uiImage: image).resizable().scaledToFill()
                } else {
                    Text(title)
                        .font(.caption2)
                        .foregroundStyle(palette.textMuted)
                        .multilineTextAlignment(.center)
                        .padding(4)
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm, style: .continuous))
            .task(id: path) {
                guard let path else { return }
                image = await Task.detached { UIImage(contentsOfFile: path) }.value
            }
    }
}
