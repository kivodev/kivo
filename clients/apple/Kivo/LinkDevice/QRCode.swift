import CoreImage.CIFilterBuiltins
import SwiftUI

/// A QR code of a text, drawn sharp at any size, dark on white so any
/// camera reads it in either theme.
struct QRCode: View {
    let text: String

    var body: some View {
        if let image = Self.image(of: text) {
            Image(uiImage: image)
                .interpolation(.none)
                .resizable()
                .scaledToFit()
                .padding(12)
                .background(.white, in: RoundedRectangle(cornerRadius: 12))
                .accessibilityLabel(Text("QR code"))
        }
    }

    private static func image(of text: String) -> UIImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(text.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage,
              let cgImage = CIContext().createCGImage(output, from: output.extent)
        else { return nil }
        return UIImage(cgImage: cgImage)
    }
}
