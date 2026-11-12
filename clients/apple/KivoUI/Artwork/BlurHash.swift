import CoreGraphics
import Foundation

/// Decodes a BlurHash (blurha.sh): the placeholder the server sends with
/// each image, drawn while the image loads so artwork is never a grey box,
/// as a small image or just its average colour.
public enum BlurHash {
    nonisolated(unsafe) private static let cache = NSCache<NSString, CGImage>()

    /// The hash decoded, from the cache, so a page's placeholder is decoded
    /// once; decoded here and now when it isn't there yet.
    static func cached(_ hash: String) -> CGImage? {
        if let hit = cache.object(forKey: hash as NSString) { return hit }
        guard let decoded = image(hash) else { return nil }
        cache.setObject(decoded, forKey: hash as NSString)
        return decoded
    }

    /// The image's average colour, the hash's first component, read without
    /// decoding the rest; nil for a malformed hash.
    public static func averageColor(_ hash: String) -> CGColor? {
        let chars = Array(hash)
        guard chars.count >= 6, let sizeFlag = decode83(chars[0...0]),
              chars.count == 4 + 2 * (sizeFlag / 9 + 1) * (sizeFlag % 9 + 1),
              let dc = decode83(chars[2..<6])
        else { return nil }
        func channel(_ shift: Int) -> CGFloat { CGFloat((dc >> shift) & 255) / 255 }
        return CGColor(srgbRed: channel(16), green: channel(8), blue: channel(0), alpha: 1)
    }

    /// The hash as a `width` by `height` image; nil for a malformed hash.
    public static func image(_ hash: String, width: Int = 32, height: Int = 32) -> CGImage? {
        let chars = Array(hash)
        guard chars.count >= 6, let sizeFlag = decode83(chars[0...0]) else { return nil }
        let componentsY = sizeFlag / 9 + 1
        let componentsX = sizeFlag % 9 + 1
        guard chars.count == 4 + 2 * componentsX * componentsY,
              let quantisedMaximum = decode83(chars[1...1]),
              let dc = decode83(chars[2..<6])
        else { return nil }
        let maximum = Float(quantisedMaximum + 1) / 166

        var colors: [(Float, Float, Float)] = [decodeDC(dc)]
        for i in 1..<(componentsX * componentsY) {
            guard let ac = decode83(chars[(4 + i * 2)..<(6 + i * 2)]) else { return nil }
            colors.append(decodeAC(ac, maximum: maximum))
        }

        // Each basis function's cosines along a row and down a column, worked out once.
        let cosX = (0..<width).flatMap { x in (0..<componentsX).map { i in cos(Float.pi * Float(x * i) / Float(width)) } }
        let cosY = (0..<height).flatMap { y in (0..<componentsY).map { j in cos(Float.pi * Float(y * j) / Float(height)) } }
        var pixels = [UInt8](repeating: 0, count: width * height * 3)
        for y in 0..<height {
            for x in 0..<width {
                var r: Float = 0, g: Float = 0, b: Float = 0
                for j in 0..<componentsY {
                    for i in 0..<componentsX {
                        let basis = cosX[x * componentsX + i] * cosY[y * componentsY + j]
                        let color = colors[i + j * componentsX]
                        r += color.0 * basis
                        g += color.1 * basis
                        b += color.2 * basis
                    }
                }
                let offset = (y * width + x) * 3
                pixels[offset] = linearToSRGB(r)
                pixels[offset + 1] = linearToSRGB(g)
                pixels[offset + 2] = linearToSRGB(b)
            }
        }

        guard let provider = CGDataProvider(data: Data(pixels) as CFData) else { return nil }
        return CGImage(
            width: width,
            height: height,
            bitsPerComponent: 8,
            bitsPerPixel: 24,
            bytesPerRow: width * 3,
            space: CGColorSpaceCreateDeviceRGB(),
            bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.none.rawValue),
            provider: provider,
            decode: nil,
            shouldInterpolate: true,
            intent: .defaultIntent
        )
    }

    private static let alphabet = Array("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~")
    private static let digits: [Character: Int] = Dictionary(uniqueKeysWithValues: alphabet.enumerated().map { ($1, $0) })

    private static func decode83<S: Sequence>(_ chars: S) -> Int? where S.Element == Character {
        var value = 0
        for char in chars {
            guard let digit = digits[char] else { return nil }
            value = value * 83 + digit
        }
        return value
    }

    private static func decodeDC(_ value: Int) -> (Float, Float, Float) {
        (sRGBToLinear(value >> 16), sRGBToLinear((value >> 8) & 255), sRGBToLinear(value & 255))
    }

    private static func decodeAC(_ value: Int, maximum: Float) -> (Float, Float, Float) {
        func component(_ quantised: Int) -> Float {
            let v = Float(quantised - 9) / 9
            return copysign(pow(abs(v), 2), v) * maximum
        }
        return (component(value / (19 * 19)), component((value / 19) % 19), component(value % 19))
    }

    private static func sRGBToLinear(_ value: Int) -> Float {
        let v = Float(value) / 255
        return v <= 0.04045 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4)
    }

    private static func linearToSRGB(_ value: Float) -> UInt8 {
        let v = max(0, min(1, value))
        let srgb = v <= 0.0031308 ? v * 12.92 : 1.055 * pow(v, 1 / 2.4) - 0.055
        return UInt8(max(0, min(255, (srgb * 255).rounded())))
    }
}
