import Foundation
import KivoCore
import Observation

/// Link a device, past its code: the device waiting on a code or link
/// another device shows, "Allow Living Room to sign in as Anna?" with what
/// the server saw of it, and the answer. A device signing in to another
/// server is said so.
@MainActor
@Observable
final class LinkDeviceModel {
    enum Step {
        /// The code is to come.
        case enter
        case otherServer(name: String?)
        case confirm(code: String, request: PendingDevice)
        case done(name: String, allowed: Bool)
    }

    private(set) var step = Step.enter
    private(set) var busy = false
    /// What went wrong, in a sentence.
    private(set) var message: String?

    private let core: KivoClient
    private let session: KivoSession

    init(core: KivoClient, session: KivoSession) {
        self.core = core
        self.session = session
    }

    /// Looks up the device waiting on the code in what was scanned or typed;
    /// true once it's found, or found to be another server's.
    @discardableResult
    func look(up text: String) async -> Bool {
        guard !busy else { return false }
        guard let link = LinkCode.companion.parseOrNull(text: text) else {
            message = String(localized: "That isn't a code. Codes are six letters and digits.")
            return false
        }
        if let other = link.server, other != session.savedServer.value.id {
            let name = try? await core.savedServers().first { $0.id == other }?.name
            message = nil
            step = .otherServer(name: name)
            return true
        }
        busy = true
        defer { busy = false }
        do {
            let request = try await session.link.pendingDevice(code: link.code)
            message = nil
            step = .confirm(code: link.code, request: request)
            return true
        } catch {
            message = codeFailure(error.requestProblem)
            return false
        }
    }

    /// Allows the device waiting on [code] to sign in, or doesn't.
    func answer(code: String, request: PendingDevice, allow: Bool) async {
        busy = true
        defer { busy = false }
        let link = session.link
        guard let done = try? await (allow ? link.approve(code: code) : link.deny(code: code)) else { return }
        if let failed = done.problemOrNull() {
            message = codeFailure(failed)
        } else {
            message = nil
            step = .done(name: request.name, allowed: allow)
        }
    }

    private func codeFailure(_ problem: RequestProblem) -> String {
        (problem as? RequestProblem.Refused)?.status == 404
            ? String(localized: "That code didn't work. Check the other device for a new one.")
            : problem.sentence
    }

    /// What the server saw of the asking device: "Android TV · On your home network · Just now".
    static func context(of request: PendingDevice) -> String {
        let place = request.nearby ? String(localized: "On your home network") : String(localized: "Not on your home network")
        let started = Date(timeIntervalSince1970: TimeInterval(request.startedAt.toEpochMilliseconds()) / 1000)
        let when = Date.now.timeIntervalSince(started) < 60
            ? String(localized: "Just now")
            : started.formatted(.relative(presentation: .named))
        return [Platform.name(request.platform), place, when].compactMap { $0 }.joined(separator: " · ")
    }
}

/// The platforms a client says it runs on, for people.
enum Platform {
    static func name(_ platform: ClientPlatform?) -> String? {
        switch platform {
        case .androidTv: String(localized: "Android TV")
        case .appleTv: String(localized: "Apple TV")
        case .iphone: String(localized: "iPhone")
        case .ipad: String(localized: "iPad")
        case .androidPhone: String(localized: "Android phone")
        case .browser: String(localized: "Browser")
        case .cast, .tizen, .webos, .unknown, nil: nil
        }
    }
}
