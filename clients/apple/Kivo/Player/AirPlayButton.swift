import AVKit
import KivoUI
import SwiftUI

/// The AirPlay button: Kivo's glyph, the size and weight of the controls
/// beside it and in the accent colour while the video plays on an AirPlay
/// device ([active]), under the system's route picker, which takes the tap
/// and lists the routes. The picker draws its own glyph at whatever size
/// it's given, so it's kept all but invisible; VoiceOver reads and
/// presses the picker itself.
struct AirPlayButton: View {
    var active = false
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Image(systemName: "airplay.video")
            .foregroundStyle(active ? palette.accent : .white)
            .accessibilityHidden(true)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .overlay {
                RoutePicker()
                    .opacity(0.02)
            }
    }
}

/// The system's route picker, listing the routes the video can play to.
private struct RoutePicker: UIViewRepresentable {
    func makeUIView(context: Context) -> AVRoutePickerView {
        let picker = AVRoutePickerView()
        picker.prioritizesVideoDevices = true
        // Kivo draws the glyph; the picker only takes the tap.
        picker.tintColor = .clear
        picker.activeTintColor = .clear
        picker.backgroundColor = .clear
        return picker
    }

    func updateUIView(_ picker: AVRoutePickerView, context: Context) {}
}
