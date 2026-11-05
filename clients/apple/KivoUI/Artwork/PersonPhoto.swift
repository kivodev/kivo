import KivoCore
import SwiftUI

/// A person's photo in a circle, or their initials when there's none.
public struct PersonPhoto: View {
    private let image: ImageRef?
    private let name: String
    private let size: CGFloat
    @Environment(\.kivoPalette) private var palette

    public init(_ image: ImageRef?, name: String, size: CGFloat) {
        self.image = image
        self.name = name
        self.size = size
    }

    public var body: some View {
        ZStack {
            if image != nil {
                ArtworkImage(image, title: name, pointWidth: size)
            } else {
                palette.placeholder
                Text(initials)
                    .font(.system(size: size * 0.32, weight: .semibold, design: .rounded))
                    .foregroundStyle(palette.onPlaceholder)
            }
        }
        .frame(width: size, height: size)
        .clipShape(Circle())
        .accessibilityHidden(true)
    }

    /// "TH" for Tom Hanks: the first letters of the first and last words.
    private var initials: String {
        let words = name.split(separator: " ").filter { $0.first?.isLetter == true }
        let letters = [words.first, words.count > 1 ? words.last : nil].compactMap { $0?.first }
        return String(letters).uppercased()
    }
}
