import KivoCore
import KivoUI
import SwiftUI

/// A heading for a section of a detail page.
struct SectionHeading: View {
    let title: String
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Text(title)
            .font(.title3.weight(.bold))
            .foregroundStyle(palette.text)
            .accessibilityAddTraits(.isHeader)
    }
}

/// A horizontal row of cards under a heading, for extras.
struct CardRow: View {
    let title: String
    let works: [WorkSummary]
    @Environment(\.wide) private var wide

    static let headingSpacing = KivoTokens.spacing * 3
    static let inset: CGFloat = 20

    static func cardWidth(_ style: CardStyle, wide: Bool = false) -> CGFloat {
        wide ? BrowseRow.cardWidth(style, wide: true) : style == .landscape ? 220 : 116
    }

    var body: some View {
        if !works.isEmpty {
            VStack(alignment: .leading, spacing: Self.headingSpacing) {
                SectionHeading(title: title)
                    .padding(.horizontal, Self.inset)
                ScrollView(.horizontal, showsIndicators: false) {
                    LazyHStack(alignment: .top, spacing: KivoTokens.spacing * 3) {
                        ForEach(works, id: \.id) { work in
                            let style = CardStyle.of(work)
                            WorkLink(work: work) {
                                WorkCard(work, style: style, width: Self.cardWidth(style, wide: wide))
                            }
                        }
                    }
                    .padding(.horizontal, Self.inset)
                }
                .scrollClipDisabled()
            }
            .cardRowInsets()
        }
    }
}

/// A collection the work is part of, its card opening the collection's page.
struct CollectionRow: View {
    let collection: CollectionSummary
    @Environment(\.openWork) private var openWork
    @Environment(\.wide) private var wide

    var body: some View {
        VStack(alignment: .leading, spacing: CardRow.headingSpacing) {
            SectionHeading(title: String(localized: "Part of \(collection.name)"))
                .padding(.horizontal, CardRow.inset)
            Button {
                openWork(WorkRoute(id: collection.id, page: .collection))
            } label: {
                GroupCard(collection, width: CardRow.cardWidth(.poster, wide: wide))
            }
            .buttonStyle(.plain)
            .hoverEffect(.highlight)
            .accessibilityIdentifier("work")
            .padding(.horizontal, CardRow.inset)
        }
        .cardRowInsets()
    }
}

/// Cast & Crew: faces and names, with the character, each opening the
/// person's page.
struct CastRow: View {
    let credits: [Credit]
    @Environment(\.kivoPalette) private var palette
    @Environment(\.openWork) private var openWork
    /// Each person's column, wider as the text grows.
    @ScaledMetric(relativeTo: .caption) private var faceWidth = 88.0

    var body: some View {
        if !credits.isEmpty {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
                SectionHeading(title: String(localized: "Cast & Crew"))
                    .padding(.horizontal, 20)
                ScrollView(.horizontal, showsIndicators: false) {
                    LazyHStack(alignment: .top, spacing: KivoTokens.spacing * 3) {
                        ForEach(Array(credits.enumerated()), id: \.offset) { _, credit in
                            Button {
                                openWork(WorkRoute(id: credit.personId, page: .person))
                            } label: {
                                face(credit)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 20)
                }
                .scrollClipDisabled()
            }
            .cardRowInsets()
        }
    }

    private func face(_ credit: Credit) -> some View {
        VStack(spacing: KivoTokens.spacing * 1.5) {
            PersonPhoto(credit.photo, name: credit.name, size: 80)
            VStack(spacing: 1) {
                Text(credit.name)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(palette.text)
                Text(credit.character ?? Words.role(credit.knownRole, credit.role))
                    .font(.caption)
                    .foregroundStyle(palette.textMuted)
            }
            .lineLimit(1)
            .multilineTextAlignment(.center)
        }
        .frame(width: faceWidth)
        .accessibilityElement(children: .combine)
    }
}

/// The overview, three lines until More; its skeleton until the page is read.
struct Overview: View {
    let text: String?
    @State private var expanded = false
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide
    @Environment(\.detailLoad) private var load

    var body: some View {
        if !load.loaded, load.problem == nil {
            OverviewSkeleton()
        } else if let text, !text.isEmpty {
            VStack(alignment: .leading, spacing: KivoTokens.spacing) {
                Text(text)
                    .font(.subheadline)
                    .foregroundStyle(palette.text.opacity(0.9))
                    .lineLimit(expanded ? nil : 3)
                Button {
                    withAnimation(.easeInOut(duration: 0.2)) { expanded.toggle() }
                } label: {
                    Text(expanded ? String(localized: "Less") : String(localized: "More"))
                        .tapTarget()
                }
                .font(.footnote.weight(.semibold))
                .foregroundStyle(palette.text)
                .buttonStyle(.plain)
            }
            .wideWidth(KivoTokens.Container.prose, wide: wide)
            .frame(maxWidth: .infinity, alignment: .leading)
            .detailRow()
        }
    }
}

/// An ``Overview``'s skeleton: its three lines and More.
struct OverviewSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing) {
            VStack(alignment: .leading, spacing: 0) {
                Text(verbatim: " ").font(.subheadline).skeletonLine()
                Text(verbatim: " ").font(.subheadline).skeletonLine()
                Text(verbatim: " ").font(.subheadline).skeletonLine().padding(.trailing, 80)
            }
            Text(verbatim: " ").font(.footnote.weight(.semibold)).skeletonLine(width: 36)
        }
        .detailRow()
    }
}

/// A ``CardRow``'s skeleton, its heading and the cards it starts with.
struct CardRowSkeleton: View {
    let style: CardStyle

    var body: some View {
        SkeletonRow(style: style, width: CardRow.cardWidth(style), spacing: CardRow.headingSpacing, inset: CardRow.inset) {
            SectionHeading(title: " ")
        }
        .cardRowInsets()
    }
}

extension View {
    /// The list row of a row of cards or faces.
    func cardRowInsets() -> some View {
        detailRow(EdgeInsets(top: 16, leading: 0, bottom: 8, trailing: 0))
    }
}
