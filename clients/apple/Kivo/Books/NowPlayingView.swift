import AVKit
import KivoCore
import KivoUI
import SwiftUI

/// Now Playing, a sheet over the app that a swipe down
/// closes: on a soft blur of the cover's colours, the large cover, the
/// title and "Author · read by Narrator", each name opening their page;
/// the chapter playing over a seek bar that spans it, the whole book's
/// progress under it with "Chapter 12 of 56 · 14h 2m left in the book";
/// Previous chapter, Back 10 seconds, Play or Pause, Forward 30 seconds and
/// Next chapter; then Speed, Sleep, Chapters and AirPlay.
struct NowPlayingView: View {
    let book: BookPlayer
    @Environment(ServerModel.self) private var server
    @State private var chaptersShown = false

    var body: some View {
        GeometryReader { geometry in
            VStack(spacing: 0) {
                header
                Spacer(minLength: KivoTokens.spacing * 2)
                BookCover(work: book.work, size: coverSize(geometry.size), radius: 12)
                    .shadow(color: .black.opacity(0.28), radius: 24, y: 12)
                    .scaleEffect(book.playing ? 1 : 0.9)
                    .animation(.spring(duration: 0.4, bounce: 0.25), value: book.playing)
                Spacer(minLength: KivoTokens.spacing * 4)
                titles
                Spacer(minLength: KivoTokens.spacing * 4)
                ChapterSeekBar(book: book)
                Spacer(minLength: KivoTokens.spacing * 3)
                transport
                Spacer(minLength: KivoTokens.spacing * 3)
                BottomRow(book: book, chaptersShown: $chaptersShown)
            }
            .padding(.horizontal, 28)
            .padding(.bottom, KivoTokens.spacing * 2)
            .frame(maxWidth: 560)
            .frame(maxWidth: .infinity)
        }
        .background { CoverBackground(work: book.work) }
        .presentationDragIndicator(.visible)
        .sheet(isPresented: $chaptersShown) {
            ChapterList(book: book)
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
        }
    }

    private func coverSize(_ size: CGSize) -> CGFloat {
        max(160, min(size.width - 56, size.height * 0.4))
    }

    private var header: some View {
        HStack {
            Spacer()
            Menu {
                if let work = book.work {
                    Button {
                        open(WorkRoute(work.summary()))
                    } label: {
                        Label(String(localized: "Go to book"), systemImage: "book")
                    }
                }
                Button(role: .destructive) {
                    book.stop()
                } label: {
                    Label(String(localized: "Stop listening"), systemImage: "stop.circle")
                }
            } label: {
                Image(systemName: "ellipsis")
                    .font(.system(size: 17, weight: .semibold))
                    .frame(width: 36, height: 36)
                    .background(.ultraThinMaterial, in: Circle())
            }
            .tint(.primary)
            .accessibilityLabel(Text("More"))
            .accessibilityIdentifier("nowPlayingMore")
        }
        .padding(.top, KivoTokens.spacing * 4)
    }

    private var titles: some View {
        VStack(spacing: KivoTokens.spacing) {
            Text(book.work?.title ?? "")
                .font(.title2.weight(.bold))
                .multilineTextAlignment(.center)
                .lineLimit(2)
                .minimumScaleFactor(0.8)
                .accessibilityIdentifier("nowPlayingTitle")
            if let info = book.work?.book {
                PeopleLine(authors: info.authors, narrators: info.narrators, open: open)
            }
        }
    }

    private var transport: some View {
        HStack {
            transportButton("backward.end.fill", size: 22, label: String(localized: "Previous chapter"), identifier: "previousChapter", action: book.previousChapter)
            Spacer()
            transportButton("gobackward.10", size: 28, label: String(localized: "Back 10 seconds"), identifier: "back10", action: book.skipBack)
            Spacer()
            PlayPauseButton(book: book, size: 46)
                .frame(width: 76, height: 76)
                .accessibilityIdentifier("nowPlayingPlay")
            Spacer()
            transportButton("goforward.30", size: 28, label: String(localized: "Forward 30 seconds"), identifier: "forward30", action: book.skipForward)
            Spacer()
            transportButton("forward.end.fill", size: 22, label: String(localized: "Next chapter"), identifier: "nextChapter", action: book.nextChapter)
                .disabled(!book.hasNext)
        }
    }

    private func transportButton(_ symbol: String, size: CGFloat, label: String, identifier: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: size, weight: .semibold))
                .frame(width: 52, height: 52)
                .contentShape(Rectangle())
        }
        .buttonStyle(TransportButtonStyle())
        .accessibilityLabel(Text(label))
        .accessibilityIdentifier(identifier)
    }

    /// A person's page or the book's, on the tab showing, with Now Playing closed.
    private func open(_ route: WorkRoute) {
        book.expanded = false
        if server.navigation.tab == .downloads { server.navigation.tab = .home }
        server.navigation.push(.work(route))
    }
}

/// "Andy Weir · read by Ray Porter", each name opening their page, or a
/// menu of them when there are several; on two lines when one won't do.
private struct PeopleLine: View {
    let authors: [PersonRef]
    let narrators: [PersonRef]
    let open: (WorkRoute) -> Void

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: KivoTokens.spacing) {
                names(authors)
                if !authors.isEmpty, !narrators.isEmpty { Text(verbatim: "·") }
                readBy
            }
            VStack(spacing: 2) {
                names(authors)
                readBy
            }
        }
        .font(.subheadline)
        .foregroundStyle(.secondary)
        .lineLimit(1)
    }

    @ViewBuilder private var readBy: some View {
        if !narrators.isEmpty {
            HStack(spacing: KivoTokens.spacing) {
                Text("read by")
                names(narrators)
            }
        }
    }

    @ViewBuilder private func names(_ people: [PersonRef]) -> some View {
        let label = Text(people.map(\.name).joined(separator: ", ")).fontWeight(.semibold)
        if people.count == 1, let person = people.first {
            Button { open(WorkRoute(id: person.id, page: .person)) } label: { label }
                .buttonStyle(.plain)
                .foregroundStyle(.primary.opacity(0.85))
        } else if !people.isEmpty {
            Menu {
                ForEach(people, id: \.id) { person in
                    Button(person.name) { open(WorkRoute(id: person.id, page: .person)) }
                }
            } label: {
                label
            }
            .tint(.primary.opacity(0.85))
        }
    }
}

/// The chapter's title over its seek bar, which spans only the
/// chapter playing, with the time played in it and the time left either
/// side; it seeks on release. Under it the whole book's progress as a thin
/// line, and where the chapter is in the book with the time left.
private struct ChapterSeekBar: View {
    let book: BookPlayer
    /// While a finger is on the bar: the chapter it spans and how far along it.
    @State private var drag: (chapter: Int32, fraction: Double)?

    var body: some View {
        let progress = book.progress
        let fraction = drag?.fraction ?? progress?.chapterFraction ?? 0
        let duration = progress?.chapterDurationMs ?? 0
        let elapsed = Int64(fraction * Double(duration))
        VStack(spacing: KivoTokens.spacing * 1.5) {
            Text(progress.map(BookWords.chapter) ?? " ")
                .font(.subheadline.weight(.semibold))
                .lineLimit(1)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("chapterTitle")
            GeometryReader { geometry in
                ZStack(alignment: .leading) {
                    Capsule().fill(Color.primary.opacity(0.18))
                    Capsule().fill(Color.primary.opacity(0.85))
                        .frame(width: max(0, geometry.size.width * fraction))
                }
                .frame(height: drag == nil ? 6 : 10)
                .frame(maxHeight: .infinity)
                .contentShape(Rectangle())
                .gesture(DragGesture(minimumDistance: 0)
                    .onChanged { value in
                        guard let progress else { return }
                        let at = min(1, max(0, value.location.x / max(1, geometry.size.width)))
                        drag = (drag?.chapter ?? progress.chapterIndex, Double(at))
                    }
                    .onEnded { _ in
                        if let drag {
                            book.seek(chapter: drag.chapter, offsetMs: Int64(drag.fraction * Double(duration)))
                        }
                        drag = nil
                    })
                .animation(.easeOut(duration: 0.15), value: drag == nil)
            }
            .frame(height: 22)
            .accessibilityElement()
            .accessibilityLabel(Text("Chapter position"))
            .accessibilityValue(Text(Display.app.clock(ms: elapsed)))
            .accessibilityAdjustableAction { direction in
                switch direction {
                case .increment: book.skipForward()
                case .decrement: book.skipBack()
                @unknown default: break
                }
            }
            .accessibilityIdentifier("chapterSeek")
            HStack {
                Text(Display.app.clock(ms: elapsed))
                Spacer()
                Text(verbatim: "-\(Display.app.clock(ms: duration - elapsed))")
            }
            .font(.caption.monospacedDigit().weight(.medium))
            .foregroundStyle(.secondary)
            GeometryReader { geometry in
                ZStack(alignment: .leading) {
                    Capsule().fill(Color.primary.opacity(0.14))
                    Capsule().fill(Color.primary.opacity(0.55))
                        .frame(width: geometry.size.width * (progress?.fraction ?? 0))
                }
            }
            .frame(height: 3)
            .padding(.top, KivoTokens.spacing * 2)
            .accessibilityHidden(true)
            Text(progress.map(BookWords.bookLine) ?? " ")
                .font(.caption)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("bookProgress")
        }
    }
}

/// Speed, Sleep, Chapters and AirPlay.
private struct BottomRow: View {
    let book: BookPlayer
    @Binding var chaptersShown: Bool

    var body: some View {
        HStack {
            speed
            Spacer()
            sleep
            Spacer()
            Button {
                chaptersShown = true
            } label: {
                Image(systemName: "list.bullet")
                    .font(.system(size: 19, weight: .semibold))
                    .frame(width: 64, height: 44)
            }
            .buttonStyle(TransportButtonStyle())
            .accessibilityLabel(Text("Chapters"))
            .accessibilityIdentifier("chapters")
            Spacer()
            RoutePicker()
                .frame(width: 64, height: 44)
                .accessibilityLabel(Text("AirPlay"))
        }
    }

    /// The user's speed for books, 0.5× to 3×, the same in every app.
    private var speed: some View {
        Menu {
            Picker(String(localized: "Speed"), selection: Binding(get: { book.speed }, set: { book.setSpeed($0) })) {
                ForEach(BookPlayerModel.companion.SPEEDS.map(\.doubleValue), id: \.self) { speed in
                    Text(BookWords.speed(speed)).tag(speed)
                }
            }
            .pickerStyle(.inline)
        } label: {
            Text(BookWords.speed(book.speed))
                .font(.system(size: 16, weight: .bold).monospacedDigit())
                .frame(width: 64, height: 44)
        }
        .tint(.primary)
        .accessibilityLabel(Text("Speed \(BookWords.speed(book.speed))"))
        .accessibilityIdentifier("speed")
    }

    /// The sleep timer: the time left on the button while it runs; the last choice first.
    private var sleep: some View {
        let running = book.sleep
        return Menu {
            if running != nil {
                Button(String(localized: "Off"), systemImage: "moon.zzz") { book.stopSleep() }
                Divider()
            }
            ForEach(Array(choices.enumerated()), id: \.offset) { _, choice in
                Button(BookWords.sleep(choice)) { book.setSleep(choice) }
            }
        } label: {
            Group {
                if let running {
                    if running.choice is SleepChoice.EndOfChapter {
                        Label(String(localized: "End of chapter"), systemImage: "moon.zzz.fill")
                    } else {
                        Label(Display.app.clock(ms: running.seconds * 1000), systemImage: "moon.zzz.fill")
                            .monospacedDigit()
                    }
                } else {
                    Image(systemName: "moon.zzz")
                }
            }
            .font(.system(size: running == nil ? 19 : 14, weight: .semibold))
            .labelStyle(SleepLabelStyle())
            .frame(minWidth: 64, minHeight: 44)
        }
        .tint(.primary)
        .accessibilityLabel(Text("Sleep timer"))
        .accessibilityIdentifier("sleep")
    }

    private var choices: [SleepChoice] {
        let all: [SleepChoice] = SleepChoice.companion.MINUTES.map { SleepChoice.Minutes(minutes: $0.int32Value) } + [SleepChoice.EndOfChapter.shared]
        guard let last = book.model.lastSleep else { return all }
        return [last] + all.filter { $0 != last }
    }
}

/// The moon and the time left beside it.
private struct SleepLabelStyle: LabelStyle {
    func makeBody(configuration: Configuration) -> some View {
        HStack(spacing: 5) {
            configuration.icon
            configuration.title
        }
    }
}

/// The chapters, the one playing marked; a tap plays from its start.
private struct ChapterList: View {
    let book: BookPlayer
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        NavigationStack {
            ScrollViewReader { reader in
                List {
                    ForEach(Array(book.chapters.enumerated()), id: \.offset) { index, chapter in
                        let current = book.chapterIndex.map(Int.init) == index
                        Button {
                            book.playChapter(index)
                            dismiss()
                        } label: {
                            HStack(spacing: KivoTokens.spacing * 3) {
                                Group {
                                    if current {
                                        Image(systemName: book.playing ? "waveform" : "speaker.fill")
                                            .symbolEffect(.variableColor.iterative, isActive: book.playing)
                                            .foregroundStyle(palette.accent)
                                    } else {
                                        Text(verbatim: "\(index + 1)")
                                            .foregroundStyle(.secondary)
                                    }
                                }
                                .font(.subheadline.monospacedDigit())
                                .frame(width: 28)
                                Text(BookWords.chapter(title: chapter.title, index: index))
                                    .font(.body.weight(current ? .semibold : .regular))
                                    .foregroundStyle(current ? palette.accent : palette.text)
                                    .lineLimit(2)
                                Spacer()
                                Text(Display.app.clock(ms: Int64(chapter.endMs - chapter.startMs)))
                                    .font(.subheadline.monospacedDigit())
                                    .foregroundStyle(.secondary)
                            }
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .id(index)
                        .accessibilityAddTraits(current ? .isSelected : [])
                        .accessibilityIdentifier("chapterRow")
                    }
                }
                .listStyle(.plain)
                .onAppear {
                    if let index = book.chapterIndex { reader.scrollTo(Int(index), anchor: .center) }
                }
            }
            .navigationTitle(String(localized: "Chapters"))
            .toolbarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "Done")) { dismiss() }
                }
            }
        }
    }
}

/// A soft blur of the cover's colours behind Now Playing, toned for the
/// light or dark appearance so the text over it reads.
struct CoverBackground: View {
    let work: WorkDetail?
    @Environment(\.colorScheme) private var scheme

    var body: some View {
        ZStack {
            Color(uiColor: .systemBackground)
            ArtworkImage(work?.images.poster, title: "", pointWidth: 64)
                .scaleEffect(1.6)
                .blur(radius: 70)
                .saturation(1.5)
                .opacity(scheme == .dark ? 0.85 : 0.75)
            LinearGradient(
                colors: scheme == .dark
                    ? [.black.opacity(0.15), .black.opacity(0.5)]
                    : [.white.opacity(0.25), .white.opacity(0.55)],
                startPoint: .top,
                endPoint: .bottom
            )
        }
        .ignoresSafeArea()
        .accessibilityHidden(true)
    }
}

/// The transport's glyphs dim while pressed, as the system's do.
private struct TransportButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .foregroundStyle(enabled ? Color.primary : Color.primary.opacity(0.3))
            .opacity(configuration.isPressed ? 0.45 : 1)
            .scaleEffect(configuration.isPressed ? 0.92 : 1)
            .animation(.easeOut(duration: 0.12), value: configuration.isPressed)
    }
}

/// The system's AirPlay and audio route picker.
private struct RoutePicker: UIViewRepresentable {
    func makeUIView(context: Context) -> AVRoutePickerView {
        let view = AVRoutePickerView()
        view.prioritizesVideoDevices = false
        view.tintColor = .label
        return view
    }

    func updateUIView(_ view: AVRoutePickerView, context: Context) {}
}
