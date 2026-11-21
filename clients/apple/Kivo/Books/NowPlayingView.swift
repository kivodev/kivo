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
    @Environment(\.kivoPalette) private var palette
    @Environment(\.verticalSizeClass) private var verticalSize
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @ScaledMetric(relativeTo: .title2) private var textScale: CGFloat = 1
    @State private var chaptersShown = false

    var body: some View {
        GeometryReader { geometry in
            if verticalSize == .compact {
                landscape(geometry.size)
            } else {
                // The largest cover that leaves the controls whole; at the largest text sizes it scrolls.
                ViewThatFits(in: .vertical) {
                    portrait(cover: coverSize(geometry.size))
                    portrait(cover: 160)
                    portrait(cover: 110)
                    ScrollView { portrait(cover: 110) }
                }
            }
        }
        .background { CoverBackground(work: book.work) }
        .presentationDragIndicator(.visible)
        .sheet(isPresented: $chaptersShown) {
            ChapterList(book: book)
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
        }
    }

    private func portrait(cover size: CGFloat) -> some View {
        VStack(spacing: 0) {
            header(top: KivoTokens.spacing * 4)
            Spacer(minLength: KivoTokens.spacing * 2)
            cover(size)
            Spacer(minLength: KivoTokens.spacing * 4)
            titles
            Spacer(minLength: KivoTokens.spacing * 4)
            ChapterSeekBar(book: book, chaptersShown: $chaptersShown)
            Spacer(minLength: KivoTokens.spacing * 3)
            failure
            transportRow
            Spacer(minLength: KivoTokens.spacing * 3)
            BottomRow(book: book, chaptersShown: $chaptersShown)
        }
        .padding(.horizontal, 28)
        .padding(.bottom, KivoTokens.spacing * 2)
        .frame(maxWidth: 560)
        .frame(maxWidth: .infinity)
    }

    /// A phone on its side: the cover beside the controls, which scroll if they still don't fit.
    private func landscape(_ size: CGSize) -> some View {
        HStack(spacing: 28) {
            cover(max(120, min(size.height - 40, size.width * 0.3)))
            ViewThatFits(in: .vertical) {
                landscapeControls
                ScrollView { landscapeControls }
            }
            .frame(maxWidth: 560)
        }
        .padding(.horizontal, 28)
        .padding(.vertical, KivoTokens.spacing * 2)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var landscapeControls: some View {
        VStack(spacing: 0) {
            header(top: KivoTokens.spacing * 2)
            Spacer(minLength: KivoTokens.spacing)
            titles
            Spacer(minLength: KivoTokens.spacing * 2)
            ChapterSeekBar(book: book, chaptersShown: $chaptersShown)
            Spacer(minLength: KivoTokens.spacing * 2)
            failure
            transportRow
            Spacer(minLength: KivoTokens.spacing * 2)
            BottomRow(book: book, chaptersShown: $chaptersShown, captions: false)
        }
    }

    private func coverSize(_ size: CGSize) -> CGFloat {
        max(160, min(size.width - 56, size.height * 0.4))
    }

    private func cover(_ size: CGFloat) -> some View {
        BookCover(poster: book.poster, title: book.title, size: size, radius: 12)
            .shadow(color: .black.opacity(0.28), radius: 24, y: 12)
            .scaleEffect(book.playing || reduceMotion ? 1 : 0.9)
            .animation(reduceMotion ? nil : .spring(duration: 0.4, bounce: 0.25), value: book.playing)
    }

    @ViewBuilder private var failure: some View {
        if let line = book.failureLine {
            Label(line, systemImage: "exclamationmark.circle")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.bottom, KivoTokens.spacing * 2)
                .accessibilityIdentifier("nowPlayingFailure")
        }
    }

    private var transportRow: some View {
        transport
            .overlay(alignment: .top) {
                if let choice = book.sleepRestart {
                    SleepRestartPopover(choice: choice, ends: book.sleepRestartEnds, restart: book.togglePlay)
                        .fixedSize()
                        .offset(y: -64)
                        .transition(.scale(scale: 0.9, anchor: .bottom).combined(with: .opacity))
                }
            }
            .animation(.spring(duration: 0.3), value: book.sleepRestart != nil)
    }

    private func header(top: CGFloat) -> some View {
        HStack {
            Button {
                book.expanded = false
            } label: {
                Image(systemName: "chevron.down")
                    .font(.system(size: 17, weight: .semibold))
                    .frame(width: 36, height: 36)
                    .background(.ultraThinMaterial, in: Circle())
            }
            .tint(.primary)
            .accessibilityLabel(Text("Close"))
            .accessibilityIdentifier("nowPlayingClose")
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
        .padding(.top, top)
    }

    private var titles: some View {
        VStack(spacing: KivoTokens.spacing) {
            Text(book.title)
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
            PlayPauseButton(book: book, size: 30 * glyphScale)
                .foregroundStyle(palette.onAccent)
                .tint(palette.onAccent)
                .frame(width: 72, height: 72)
                .background(palette.accent, in: Circle())
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
                .font(.system(size: size * glyphScale, weight: .semibold))
                .frame(width: 52, height: 52)
                .contentShape(Rectangle())
        }
        .buttonStyle(TransportButtonStyle())
        .accessibilityLabel(Text(label))
        .accessibilityIdentifier(identifier)
    }

    /// The transport's glyphs grow with the text, as far as five of them still fit a phone.
    private var glyphScale: CGFloat { min(textScale, 1.2) }

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
    @Binding var chaptersShown: Bool
    /// While a finger is on the bar: the chapter it spans and how far along it.
    @State private var drag: (chapter: Int32, fraction: Double)?

    var body: some View {
        let progress = book.progress
        let fraction = drag?.fraction ?? progress?.chapterFraction ?? 0
        let duration = progress?.chapterDurationMs ?? 0
        let elapsed = Int64(fraction * Double(duration))
        VStack(spacing: KivoTokens.spacing * 1.5) {
            HStack(spacing: KivoTokens.spacing) {
                Text(progress.map(BookWords.chapter) ?? " ")
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                    .accessibilityIdentifier("chapterTitle")
                    .accessibilityAction(named: Text("Show chapters")) { chaptersShown = true }
                Image(systemName: "chevron.down")
                    .font(.caption2.weight(.bold))
                    .foregroundStyle(.secondary)
                    .accessibilityHidden(true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
            .onTapGesture { chaptersShown = true }
            GeometryReader { geometry in
                ZStack(alignment: .leading) {
                    Capsule().fill(Color.primary.opacity(0.18))
                    Capsule().fill(Color.primary.opacity(0.85))
                        .frame(width: max(0, geometry.size.width * fraction))
                }
                .frame(height: drag == nil ? 6 : 10)
                .frame(maxHeight: .infinity)
                .overlay(alignment: .leading) {
                    let size: CGFloat = drag == nil ? 14 : 20
                    Circle()
                        .fill(Color.primary)
                        .frame(width: size, height: size)
                        .shadow(color: .black.opacity(0.3), radius: 2, y: 1)
                        .offset(x: geometry.size.width * fraction - size / 2)
                }
                .contentShape(Rectangle())
                .gesture(DragGesture(minimumDistance: 6)
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
            .frame(height: 44)
            .padding(.vertical, -11)
            .sensoryFeedback(.selection, trigger: drag != nil)
            .accessibilityElement()
            .accessibilityLabel(Text("Chapter position"))
            .accessibilityValue(Text("\(Display.app.clock(ms: elapsed)) of \(Display.app.clock(ms: duration))"))
            .accessibilityAdjustableAction { direction in
                guard let progress else { return }
                let step: Int64 = 15000
                switch direction {
                case .increment: book.seek(chapter: progress.chapterIndex, offsetMs: min(duration, elapsed + step))
                case .decrement: book.seek(chapter: progress.chapterIndex, offsetMs: max(0, elapsed - step))
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
            Text(progress.map { BookWords.bookLine($0, speed: book.speed) } ?? " ")
                .font(.caption)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("bookProgress")
        }
    }
}

/// Speed, Sleep, Chapters and AirPlay, each named under its glyph where there's the height for it.
private struct BottomRow: View {
    let book: BookPlayer
    @Binding var chaptersShown: Bool
    var captions = true

    var body: some View {
        HStack {
            speed
            Spacer()
            sleep
            Spacer()
            Button {
                chaptersShown = true
            } label: {
                item(Text("Chapters")) {
                    Image(systemName: "list.bullet")
                        .font(.system(size: 19, weight: .semibold))
                }
            }
            .buttonStyle(TransportButtonStyle())
            .accessibilityLabel(Text("Chapters"))
            .accessibilityIdentifier("chapters")
            Spacer()
            item(Text("AirPlay")) {
                RoutePicker()
                    .frame(width: 64)
                    .accessibilityLabel(Text("AirPlay"))
            }
        }
        .dynamicTypeSize(...DynamicTypeSize.xxLarge)
    }

    /// A control of the row, its name under it.
    private func item<Content: View>(_ caption: Text, @ViewBuilder _ content: () -> Content) -> some View {
        VStack(spacing: 2) {
            content()
                .frame(height: captions ? 28 : 44)
            if captions {
                caption
                    .font(.caption2.weight(.medium))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
                    .accessibilityHidden(true)
            }
        }
        .frame(width: 64)
        .frame(minHeight: 44)
        .contentShape(Rectangle())
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
            item(Text("Speed")) {
                Text(BookWords.speed(book.speed))
                    .font(.system(size: 16, weight: .bold).monospacedDigit())
            }
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
                Button {
                    book.setSleep(choice)
                } label: {
                    if running?.choice == choice {
                        Label(BookWords.sleep(choice), systemImage: "checkmark")
                    } else {
                        Text(BookWords.sleep(choice))
                    }
                }
            }
        } label: {
            item(Text("Sleep")) {
                Group {
                    if let running {
                        Label(Display.app.clock(ms: running.seconds * 1000), systemImage: "moon.zzz.fill")
                            .monospacedDigit()
                            .lineLimit(1)
                            .minimumScaleFactor(0.8)
                    } else {
                        Image(systemName: "moon.zzz")
                    }
                }
                .font(.system(size: running == nil ? 19 : 14, weight: .semibold))
                .labelStyle(SleepLabelStyle())
            }
        }
        .tint(.primary)
        .accessibilityLabel(Text("Sleep timer"))
        .accessibilityValue(running.map { Text("\(BookWords.sleep($0.choice)), \(Display.app.clock(ms: $0.seconds * 1000)) left") } ?? Text("Off"))
        .accessibilityIdentifier("sleep")
    }

    private var choices: [SleepChoice] {
        SleepChoice.companion.MINUTES.map { SleepChoice.Minutes(minutes: $0.int32Value) } + [SleepChoice.EndOfChapter.shared]
    }
}

/// Over Play for the 15 seconds after the sleep timer pauses the book: a
/// ring counting them down, and what a play, or a tap here, starts again.
private struct SleepRestartPopover: View {
    let choice: SleepChoice
    let ends: Date
    let restart: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button(action: restart) {
            HStack(spacing: KivoTokens.spacing * 3) {
                TimelineView(.periodic(from: .now, by: 0.1)) { context in
                    let left = max(0, ends.timeIntervalSince(context.date))
                    ZStack {
                        Circle().stroke(Color.primary.opacity(0.18), lineWidth: 3)
                        Circle()
                            .trim(from: 0, to: left / window)
                            .stroke(palette.accent, style: StrokeStyle(lineWidth: 3, lineCap: .round))
                            .rotationEffect(.degrees(-90))
                        Text(verbatim: "\(Int(left.rounded(.up)))")
                            .font(.subheadline.monospacedDigit().weight(.bold))
                    }
                    .frame(width: 36, height: 36)
                }
                VStack(alignment: .leading, spacing: 1) {
                    Text("Sleep timer ended")
                        .font(.subheadline.weight(.semibold))
                    Text("Play for \(BookWords.sleepAgain(choice))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .padding(.leading, KivoTokens.spacing * 3)
            .padding(.trailing, KivoTokens.spacing * 5)
            .padding(.vertical, KivoTokens.spacing * 2.5)
            .background(.regularMaterial, in: Capsule())
            .shadow(color: .black.opacity(0.25), radius: 16, y: 6)
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Sleep timer ended. Play for \(BookWords.sleepAgain(choice))"))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("sleepRestart")
    }

    private var window: Double { Double(BookPlayerModel.companion.SLEEP_RESTART_MS) / 1000 }
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
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

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
                                            .symbolEffect(.variableColor.iterative, isActive: book.playing && !reduceMotion)
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
