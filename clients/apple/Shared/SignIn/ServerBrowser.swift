import Foundation
import KivoCore
import Network
import Observation

/// Finds servers on the local network by Bonjour: browses for `_kivo._tcp`,
/// resolves each service to an IPv4 address, and keeps it once the server
/// there answers with the ID its TXT record names. The first browse asks
/// for Local Network access; refused, it says so in `denied`.
@MainActor
@Observable
final class ServerBrowser {
    static let serviceType = "_kivo._tcp"

    /// The servers found and confirmed, in the order found.
    private(set) var found: [FoundServer] = []
    /// Whether Local Network access was refused, so nothing can be found.
    private(set) var denied = false

    private let core: KivoClient
    @ObservationIgnored private var browser: NWBrowser?
    /// The services being resolved or already confirmed, by name.
    @ObservationIgnored private var seen: Set<String> = []

    init(core: KivoClient) {
        self.core = core
    }

    /// Starts browsing, until `stop`.
    func start() {
        guard browser == nil else { return }
        denied = false
        let browser = NWBrowser(for: .bonjourWithTXTRecord(type: Self.serviceType, domain: nil), using: .tcp)
        browser.stateUpdateHandler = { [weak self] state in
            let refused: Bool
            switch state {
            case let .waiting(error), let .failed(error): refused = error.isLocalNetworkDenied
            default: refused = false
            }
            Task { @MainActor in if refused { self?.denied = true } }
        }
        browser.browseResultsChangedHandler = { [weak self] results, _ in
            Task { @MainActor in self?.resolve(results) }
        }
        browser.start(queue: .main)
        self.browser = browser
    }

    func stop() {
        browser?.cancel()
        browser = nil
    }

    private func resolve(_ results: Set<NWBrowser.Result>) {
        for result in results {
            guard case let .service(name, _, _, _) = result.endpoint, !seen.contains(name),
                  case let .bonjour(txt) = result.metadata, let id = txt["id"]
            else { continue }
            seen.insert(name)
            let tls = txt["tls"] == "1"
            let path = txt["path"] ?? ""
            Task {
                guard let (host, port) = await Self.address(of: result.endpoint),
                      let server = try? await core.confirmDiscovered(id: id, hosts: [host], port: Int32(port), tls: tls, path: path),
                      !found.contains(where: { $0.info.id == server.info.id })
                else {
                    seen.remove(name)
                    return
                }
                found.append(server)
            }
        }
    }

    /// The IPv4 address and port a service resolves to, by connecting to it
    /// for a moment; nil when it doesn't resolve within 5 seconds.
    private static func address(of endpoint: NWEndpoint) async -> (String, UInt16)? {
        let parameters = NWParameters.tcp
        if let ip = parameters.defaultProtocolStack.internetProtocol as? NWProtocolIP.Options {
            ip.version = .v4
        }
        let connection = NWConnection(to: endpoint, using: parameters)
        return await withCheckedContinuation { continuation in
            var resumed = false
            let finish: ((String, UInt16)?) -> Void = { address in
                guard !resumed else { return }
                resumed = true
                connection.cancel()
                continuation.resume(returning: address)
            }
            connection.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    if case let .hostPort(host, port) = connection.currentPath?.remoteEndpoint, case let .ipv4(ip) = host {
                        // An address may name its interface ("%en0"), which a URL can't hold.
                        finish((String("\(ip)".prefix { $0 != "%" }), port.rawValue))
                    } else {
                        finish(nil)
                    }
                case .failed, .cancelled:
                    finish(nil)
                default:
                    break
                }
            }
            connection.start(queue: .main)
            DispatchQueue.main.asyncAfter(deadline: .now() + 5) { finish(nil) }
        }
    }
}

private extension NWError {
    /// Whether this is Local Network access refused (kDNSServiceErr_PolicyDenied).
    var isLocalNetworkDenied: Bool {
        if case let .dns(code) = self { return code == DNSServiceErrorType(kDNSServiceErr_PolicyDenied) }
        return false
    }
}
