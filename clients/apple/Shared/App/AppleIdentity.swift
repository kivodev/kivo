import Foundation
import KivoCore
import UIKit

/// The app and device as every request to a server says them: the app's
/// version, iPhone, iPad or Apple TV, the system's version and the machine
/// identifier ("iPhone16,2"). Never the device's name or its screen.
final class AppleIdentity: NSObject, ClientIdentity {
    let app = "kivo-apple"
    let appVersion: String
    let platform: ClientPlatform
    let osVersion: String
    let deviceModel: String

    init(appVersion: String, platform: ClientPlatform, osVersion: String, deviceModel: String) {
        self.appVersion = appVersion
        self.platform = platform
        self.osVersion = osVersion
        self.deviceModel = deviceModel
    }

    /// This app on this device.
    @MainActor
    static var current: AppleIdentity {
        AppleIdentity(
            appVersion: version(Bundle.main.infoDictionary ?? [:]),
            platform: platform,
            osVersion: UIDevice.current.systemVersion,
            deviceModel: machine
        )
    }

    /// What the app runs on, by the device's idiom.
    @MainActor
    private static var platform: ClientPlatform {
        switch UIDevice.current.userInterfaceIdiom {
        case .pad: .ipad
        case .tv: .appleTv
        default: .iphone
        }
    }

    /// The app's version as its requests say it: the marketing version with
    /// the build number as build metadata, such as "0.3.0+78", from the
    /// bundle's `info`.
    static func version(_ info: [String: Any]) -> String {
        let version = info["CFBundleShortVersionString"] as? String ?? ""
        guard let build = info["CFBundleVersion"] as? String, !build.isEmpty, !version.isEmpty else { return version }
        return "\(version)+\(build)"
    }

    /// The device's model by its marketing name, such as "iPhone 17 Pro",
    /// which names its sessions: the machine identifier's, the simulated
    /// device's in the Simulator, else "iPhone", "iPad" or "Apple TV".
    @MainActor
    static var modelName: String {
        let identifier = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] ?? machine
        return modelNames[identifier] ?? UIDevice.current.model
    }

    /// The machine identifier the system reports, such as "iPhone16,2".
    static var machine: String {
        var system = utsname()
        uname(&system)
        return withUnsafeBytes(of: &system.machine) { bytes in
            String(decoding: bytes.prefix { $0 != 0 }, as: UTF8.self)
        }
    }

    /// The marketing names of the models the app runs on lately, by machine identifier.
    private static let modelNames = [
        "iPhone13,1": "iPhone 12 mini", "iPhone13,2": "iPhone 12", "iPhone13,3": "iPhone 12 Pro", "iPhone13,4": "iPhone 12 Pro Max",
        "iPhone14,4": "iPhone 13 mini", "iPhone14,5": "iPhone 13", "iPhone14,2": "iPhone 13 Pro", "iPhone14,3": "iPhone 13 Pro Max",
        "iPhone14,6": "iPhone SE (3rd generation)",
        "iPhone14,7": "iPhone 14", "iPhone14,8": "iPhone 14 Plus", "iPhone15,2": "iPhone 14 Pro", "iPhone15,3": "iPhone 14 Pro Max",
        "iPhone15,4": "iPhone 15", "iPhone15,5": "iPhone 15 Plus", "iPhone16,1": "iPhone 15 Pro", "iPhone16,2": "iPhone 15 Pro Max",
        "iPhone17,3": "iPhone 16", "iPhone17,4": "iPhone 16 Plus", "iPhone17,1": "iPhone 16 Pro", "iPhone17,2": "iPhone 16 Pro Max",
        "iPhone17,5": "iPhone 16e",
        "iPhone18,3": "iPhone 17", "iPhone18,4": "iPhone Air", "iPhone18,1": "iPhone 17 Pro", "iPhone18,2": "iPhone 17 Pro Max",
        "iPhone18,5": "iPhone 17e",
        "iPhone19,2": "iPhone 18 Pro", "iPhone19,3": "iPhone 18 Pro Max", "iPhone19,7": "iPhone 18 Pro Max", "iPhone19,4": "iPhone Duo",
        "iPad13,18": "iPad (10th generation)", "iPad13,19": "iPad (10th generation)",
        "iPad15,7": "iPad (A16)", "iPad15,8": "iPad (A16)",
        "iPad16,1": "iPad mini (A17 Pro)", "iPad16,2": "iPad mini (A17 Pro)",
        "iPad19,1": "iPad mini (A20 Pro)", "iPad19,2": "iPad mini (A20 Pro)",
        "iPad14,8": "iPad Air 11-inch (M2)", "iPad14,9": "iPad Air 11-inch (M2)",
        "iPad14,10": "iPad Air 13-inch (M2)", "iPad14,11": "iPad Air 13-inch (M2)",
        "iPad15,3": "iPad Air 11-inch (M3)", "iPad15,4": "iPad Air 11-inch (M3)",
        "iPad15,5": "iPad Air 13-inch (M3)", "iPad15,6": "iPad Air 13-inch (M3)",
        "iPad16,8": "iPad Air 11-inch (M4)", "iPad16,9": "iPad Air 11-inch (M4)",
        "iPad16,10": "iPad Air 13-inch (M4)", "iPad16,11": "iPad Air 13-inch (M4)",
        "iPad16,3": "iPad Pro 11-inch (M4)", "iPad16,4": "iPad Pro 11-inch (M4)",
        "iPad16,5": "iPad Pro 13-inch (M4)", "iPad16,6": "iPad Pro 13-inch (M4)",
        "iPad17,1": "iPad Pro 11-inch (M5)", "iPad17,2": "iPad Pro 11-inch (M5)",
        "iPad17,3": "iPad Pro 13-inch (M5)", "iPad17,4": "iPad Pro 13-inch (M5)",
        "AppleTV5,3": "Apple TV HD", "AppleTV6,2": "Apple TV 4K", "AppleTV11,1": "Apple TV 4K (2nd generation)",
        "AppleTV14,1": "Apple TV 4K (3rd generation)",
    ]
}
