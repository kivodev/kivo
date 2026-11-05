import KivoUI
import SwiftUI

/// The Cast button, with Google's glyph and states: hidden with no Cast
/// device found, as off Wi-Fi; the outline when not connected; the arcs
/// lighting in turn while connecting; and filled in the accent colour once
/// connected. Not connected, it opens Google's list of devices; connected,
/// the Cast menu.
struct CastButton: View {
    let cast: CastModel
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        if cast.buttonShown {
            Button(action: cast.buttonTapped) {
                CastGlyph(connection: cast.connection)
                    .frame(width: 22, height: 22)
                    .foregroundStyle(cast.connection == .connected ? AnyShapeStyle(palette.accent) : AnyShapeStyle(.tint))
            }
            .accessibilityLabel(Text("Cast"))
            .accessibilityValue(value)
            .accessibilityIdentifier("cast")
        }
    }

    private var value: Text {
        switch cast.connection {
        case .disconnected: Text("Not connected")
        case .connecting: Text("Connecting")
        case .connected: Text(cast.device ?? "")
        }
    }
}

/// Google's Cast glyph: a screen with the arcs of a signal in its corner,
/// and the screen filled once connected. Connecting, the arcs light one
/// at a time, from the dot out.
struct CastGlyph: View {
    let connection: CastModel.Connection

    var body: some View {
        if connection == .connecting {
            TimelineView(.periodic(from: .now, by: 0.4)) { context in
                let lit = Int(context.date.timeIntervalSinceReferenceDate / 0.4) % 3
                glyph(lit: lit)
            }
        } else {
            glyph(lit: nil)
        }
    }

    /// Arc [lit] at full strength and the others faint; all of them with none.
    private func glyph(lit: Int?) -> some View {
        ZStack {
            CastShape(part: .screen)
            ForEach(0..<3) { arc in
                CastShape(part: .arc(arc)).opacity(lit.map { arc == $0 ? 1 : 0.3 } ?? 1)
            }
            if connection == .connected { CastShape(part: .fill) }
        }
    }
}

/// A part of the Cast glyph on its 24-point grid.
private struct CastShape: Shape {
    enum Part {
        case screen
        /// The dot (0) and the two arcs around it (1, 2).
        case arc(Int)
        /// The screen's inside, connected.
        case fill
    }

    let part: Part

    func path(in rect: CGRect) -> Path {
        let s = min(rect.width, rect.height) / 24
        func p(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: rect.minX + x * s, y: rect.minY + y * s) }
        var path = Path()
        switch part {
        case .screen:
            path.move(to: p(21, 3))
            path.addLine(to: p(3, 3))
            path.addCurve(to: p(1, 5), control1: p(1.9, 3), control2: p(1, 3.9))
            path.addLine(to: p(1, 8))
            path.addLine(to: p(3, 8))
            path.addLine(to: p(3, 5))
            path.addLine(to: p(21, 5))
            path.addLine(to: p(21, 19))
            path.addLine(to: p(14, 19))
            path.addLine(to: p(14, 21))
            path.addLine(to: p(21, 21))
            path.addCurve(to: p(23, 19), control1: p(22.1, 21), control2: p(23, 20.1))
            path.addLine(to: p(23, 5))
            path.addCurve(to: p(21, 3), control1: p(23, 3.9), control2: p(22.1, 3))
        case .arc(0):
            path.move(to: p(1, 18))
            path.addLine(to: p(1, 21))
            path.addLine(to: p(4, 21))
            path.addCurve(to: p(1, 18), control1: p(4, 19.34), control2: p(2.66, 18))
        case .arc(1):
            path.move(to: p(1, 14))
            path.addLine(to: p(1, 16))
            path.addCurve(to: p(6, 21), control1: p(3.76, 16), control2: p(6, 18.24))
            path.addLine(to: p(8, 21))
            path.addCurve(to: p(1, 14), control1: p(8, 17.13), control2: p(4.87, 14))
        case .arc:
            path.move(to: p(1, 10))
            path.addLine(to: p(1, 12))
            path.addCurve(to: p(10, 21), control1: p(5.97, 12), control2: p(10, 16.03))
            path.addLine(to: p(12, 21))
            path.addCurve(to: p(1, 10), control1: p(12, 14.92), control2: p(7.07, 10))
        case .fill:
            path.move(to: p(19, 7))
            path.addLine(to: p(5, 7))
            path.addLine(to: p(5, 8.63))
            path.addCurve(to: p(13.35, 16.98), control1: p(8.96, 9.91), control2: p(12.07, 13.02))
            path.addLine(to: p(19, 16.98))
        }
        path.closeSubpath()
        return path
    }
}
