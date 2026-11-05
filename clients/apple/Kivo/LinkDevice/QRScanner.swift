import AVFoundation
import SwiftUI

/// The camera, reading QR codes, until one is read.
struct QRScanner: UIViewControllerRepresentable {
    let scanned: (String) -> Void

    /// Whether there's a camera to scan with: none in the Simulator.
    static var available: Bool {
        AVCaptureDevice.default(for: .video) != nil
    }

    /// What UI tests have the scanner read, by the launch argument
    /// `-KivoScan <text>`, since the Simulator has no camera.
    static var testScan: String? {
        UserDefaults.standard.string(forKey: "KivoScan")
    }

    func makeUIViewController(context: Context) -> ScannerController {
        let controller = ScannerController()
        controller.scanned = scanned
        return controller
    }

    func updateUIViewController(_ controller: ScannerController, context: Context) {
        controller.scanned = scanned
    }

    final class ScannerController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
        var scanned: ((String) -> Void)?
        private let session = AVCaptureSession()
        private let preview = AVCaptureVideoPreviewLayer()
        /// The last code read, so holding the camera on it reads it once.
        private var last: String?

        override func viewDidLoad() {
            super.viewDidLoad()
            view.backgroundColor = .black
            preview.session = session
            preview.videoGravity = .resizeAspectFill
            view.layer.addSublayer(preview)
            AVCaptureDevice.requestAccess(for: .video) { granted in
                guard granted else { return }
                DispatchQueue.main.async { self.configure() }
            }
        }

        override func viewDidLayoutSubviews() {
            super.viewDidLayoutSubviews()
            preview.frame = view.bounds
        }

        override func viewWillDisappear(_ animated: Bool) {
            super.viewWillDisappear(animated)
            let session = session
            DispatchQueue.global(qos: .userInitiated).async { session.stopRunning() }
        }

        private func configure() {
            guard let camera = AVCaptureDevice.default(for: .video),
                  let input = try? AVCaptureDeviceInput(device: camera),
                  session.canAddInput(input)
            else { return }
            session.addInput(input)
            let output = AVCaptureMetadataOutput()
            guard session.canAddOutput(output) else { return }
            session.addOutput(output)
            output.setMetadataObjectsDelegate(self, queue: .main)
            output.metadataObjectTypes = [.qr]
            let session = session
            DispatchQueue.global(qos: .userInitiated).async { session.startRunning() }
        }

        func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput objects: [AVMetadataObject], from connection: AVCaptureConnection) {
            guard let code = objects.compactMap({ ($0 as? AVMetadataMachineReadableCodeObject)?.stringValue }).first, code != last else { return }
            last = code
            scanned?(code)
        }
    }
}
