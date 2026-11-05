import Foundation
import KivoCore
import Network
import os

/// The device's network, told to the core each time it changes, as to
/// another Wi-Fi or a VPN: whether there is one, and whether it's cellular
/// or another expensive one, such as a personal hotspot, for "Waiting for
/// network" and "Waiting for Wi-Fi"; and the change itself, on which the
/// session picks its server's address again. The monitor also reports
/// paths on the same network as the last, now and then, which aren't told:
/// each change told ends the trust in a plain-HTTP address.
final class Connectivity: Sendable {
    private let monitor = NWPathMonitor()
    private let last = OSAllocatedUnfairLock<Attachment?>(initialState: nil)

    init(core: KivoClient) {
        monitor.pathUpdateHandler = { [last] path in
            let now = Attachment(path)
            let changed = last.withLock { last in
                defer { last = now }
                return now != last
            }
            if changed { core.networkChanged(online: now.online, expensive: now.expensive) }
        }
        monitor.start(queue: DispatchQueue(label: "com.kivo.player.connectivity"))
    }

    deinit {
        monitor.cancel()
    }
}

/// What makes one network another: whether there is one, whether it's
/// expensive, and the interfaces and gateways it goes through.
private struct Attachment: Equatable, Sendable {
    let online: Bool
    let expensive: Bool
    let interfaces: [String]
    let gateways: [NWEndpoint]

    init(_ path: NWPath) {
        online = path.status == .satisfied
        expensive = path.isExpensive
        interfaces = path.availableInterfaces.map(\.name)
        gateways = path.gateways
    }
}
