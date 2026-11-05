import KivoCore
import KivoUI
import SwiftUI

/// A book's square cover, rounded, or a book on the placeholder colour without one.
struct BookCover: View {
    let work: WorkDetail?
    let size: CGFloat
    var radius: CGFloat = 6
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Group {
            if let poster = work?.images.poster {
                ArtworkImage(poster, title: work?.title ?? "", pointWidth: size)
            } else {
                ZStack {
                    palette.placeholder
                    Image(systemName: "book.closed.fill")
                        .font(.system(size: size * 0.36))
                        .foregroundStyle(palette.onPlaceholder)
                }
            }
        }
        .frame(width: size, height: size)
        .clipShape(RoundedRectangle(cornerRadius: radius, style: .continuous))
        .accessibilityHidden(true)
    }
}
