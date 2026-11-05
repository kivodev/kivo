import Foundation
import GoogleCast
import KivoCore

/// The Cast session as the cast model drives it: Google's SDK, or in a
/// debug build a stand-in for the Simulator, which finds no Cast devices.
@MainActor
protocol CastLink: AnyObject {
    var delegate: CastLinkDelegate? { get set }
    /// Where the receiver's media is now, between status updates.
    var positionSeconds: Double { get }
    /// The system's list of Cast devices to connect to.
    func showDevices()
    /// [image] is the work's artwork, for Google Home and the phone's cast notification.
    func load(_ load: CastLoad, title: String, image: URL?)
    func play()
    func pause()
    func seek(to seconds: Double)
    func setRate(_ rate: Float)
    func setVolume(_ volume: Float)
    /// A message on Kivo's namespace.
    func send(_ message: String)
    /// Ends the session: [stopCasting] stops the TV too, else it plays on.
    func end(stopCasting: Bool)
    /// Stops following the session, which carries on: the app signed out.
    func close()
}

@MainActor
protocol CastLinkDelegate: AnyObject {
    func linkChanged(_ state: CastLinkState, device: String?)
    func volumeChanged(_ volume: Float)
    func mediaChanged(_ media: CastMedia?)
    func received(_ message: String)
}

/// Whether Cast devices are found, and the session with one.
enum CastLinkState: Equatable {
    /// None found, as off Wi-Fi.
    case unavailable
    case disconnected
    case connecting
    /// Connected, newly or again after the app was away: [resumed] says which.
    case connected(resumed: Bool)
}

/// The receiver's media: what plays, where, and its status.
struct CastMedia: Equatable {
    enum State {
        case loading
        case buffering
        case playing
        case paused
    }

    let workId: String
    let state: State
    let positionMs: Int64
    let durationMs: Int64
    let rate: Float
    /// The receiver's `customData`, as JSON, which the core reads.
    let status: String?
}

/// Google Cast: the shared context set up with Kivo's receiver, its session
/// manager and the session's media client, and Kivo's namespace.
final class GoogleCastLink: NSObject, CastLink {
    weak var delegate: CastLinkDelegate?
    private var channel: KivoChannel?
    private var client: GCKRemoteMediaClient?

    /// Sets up the shared context once, at launch: devices are looked for
    /// from the start, since sign-in already asked for the local network,
    /// and the hardware volume buttons drive the TV.
    static func configure() {
        let options = GCKCastOptions(discoveryCriteria: GCKDiscoveryCriteria(applicationID: Cast.shared.APP_ID))
        options.startDiscoveryAfterFirstTapOnCastButton = false
        options.physicalVolumeButtonsWillControlDeviceVolume = true
        GCKCastContext.setSharedInstanceWith(options)
    }

    override init() {
        super.init()
        let context = GCKCastContext.sharedInstance()
        context.sessionManager.add(self)
        NotificationCenter.default.addObserver(self, selector: #selector(castStateChanged), name: .gckCastStateDidChange, object: context)
        if let session = context.sessionManager.currentCastSession { attach(session) }
    }

    var positionSeconds: Double { client?.approximateStreamPosition() ?? 0 }

    func start() {
        castStateChanged()
        if let status = client?.mediaStatus { mediaChanged(status) }
    }

    func showDevices() {
        GCKCastContext.sharedInstance().presentCastDialog()
    }

    func load(_ load: CastLoad, title: String, image: URL?) {
        guard let client else { return }
        let metadata = GCKMediaMetadata(metadataType: .movie)
        metadata.setString(title, forKey: kGCKMetadataKeyTitle)
        if let image { metadata.addImage(GCKImage(url: image, width: 480, height: 720)) }
        let media = GCKMediaInformationBuilder()
        media.contentID = load.contentId
        media.entity = load.entity
        media.streamType = .buffered
        media.metadata = metadata
        media.customData = try? JSONSerialization.jsonObject(with: Data(load.customData.utf8))
        let request = GCKMediaLoadRequestDataBuilder()
        request.mediaInformation = media.build()
        request.startTime = load.currentTimeSec?.doubleValue ?? kGCKInvalidTimeInterval
        request.autoplay = NSNumber(value: true)
        client.loadMedia(with: request.build())
    }

    func play() { client?.play() }

    func pause() { client?.pause() }

    func seek(to seconds: Double) {
        let options = GCKMediaSeekOptions()
        options.interval = seconds
        client?.seek(with: options)
    }

    func setRate(_ rate: Float) { client?.setPlaybackRate(rate) }

    func setVolume(_ volume: Float) {
        GCKCastContext.sharedInstance().sessionManager.currentCastSession?.setDeviceVolume(volume)
    }

    func send(_ message: String) {
        channel?.sendTextMessage(message, error: nil)
    }

    func end(stopCasting: Bool) {
        GCKCastContext.sharedInstance().sessionManager.endSessionAndStopCasting(stopCasting)
    }

    func close() {
        let context = GCKCastContext.sharedInstance()
        context.sessionManager.remove(self)
        NotificationCenter.default.removeObserver(self)
        client?.remove(self)
        delegate = nil
    }

    @objc private func castStateChanged() {
        let context = GCKCastContext.sharedInstance()
        let name = context.sessionManager.currentCastSession?.device.friendlyName
        switch context.castState {
        case .noDevicesAvailable: delegate?.linkChanged(.unavailable, device: nil)
        case .notConnected: delegate?.linkChanged(.disconnected, device: nil)
        case .connecting: delegate?.linkChanged(.connecting, device: name)
        case .connected: delegate?.linkChanged(.connected(resumed: false), device: name)
        @unknown default: delegate?.linkChanged(.disconnected, device: nil)
        }
    }

    private func attach(_ session: GCKCastSession) {
        let channel = KivoChannel(namespace: Cast.shared.NAMESPACE)
        channel.received = { [weak self] in self?.delegate?.received($0) }
        session.add(channel)
        self.channel = channel
        client = session.remoteMediaClient
        client?.add(self)
        delegate?.volumeChanged(session.currentDeviceVolume)
    }

    private func detach() {
        client?.remove(self)
        client = nil
        channel = nil
        delegate?.mediaChanged(nil)
    }

    private func mediaChanged(_ status: GCKMediaStatus?) {
        guard let status, let info = status.mediaInformation, let workId = info.contentID else {
            delegate?.mediaChanged(nil)
            return
        }
        let state: CastMedia.State
        switch status.playerState {
        case .playing: state = .playing
        case .paused: state = .paused
        case .buffering: state = .buffering
        case .loading: state = .loading
        case .idle, .unknown:
            // Idle with media is a load under way; finished, stopped or failed leaves nothing playing.
            guard status.idleReason == GCKMediaPlayerIdleReason.none else {
                delegate?.mediaChanged(nil)
                return
            }
            state = .loading
        @unknown default: state = .loading
        }
        let custom = status.customData
            .flatMap { JSONSerialization.isValidJSONObject($0) ? try? JSONSerialization.data(withJSONObject: $0) : nil }
            .map { String(decoding: $0, as: UTF8.self) }
        delegate?.mediaChanged(CastMedia(
            workId: workId,
            state: state,
            positionMs: Int64(status.streamPosition * 1000),
            durationMs: Int64(max(info.streamDuration, 0) * 1000),
            rate: status.playbackRate,
            status: custom
        ))
    }
}

extension GoogleCastLink: GCKSessionManagerListener {
    func sessionManager(_ sessionManager: GCKSessionManager, didStart session: GCKCastSession) {
        attach(session)
        delegate?.linkChanged(.connected(resumed: false), device: session.device.friendlyName)
    }

    func sessionManager(_ sessionManager: GCKSessionManager, didResumeCastSession session: GCKCastSession) {
        attach(session)
        delegate?.linkChanged(.connected(resumed: true), device: session.device.friendlyName)
    }

    func sessionManager(_ sessionManager: GCKSessionManager, didEnd session: GCKCastSession, withError error: Error?) {
        detach()
        castStateChanged()
    }

    func sessionManager(_ sessionManager: GCKSessionManager, didFailToStart session: GCKCastSession, withError error: Error) {
        detach()
        castStateChanged()
    }

    func sessionManager(_ sessionManager: GCKSessionManager, castSession session: GCKCastSession, didReceiveDeviceVolume volume: Float, muted: Bool) {
        delegate?.volumeChanged(muted ? 0 : volume)
    }
}

extension GoogleCastLink: GCKRemoteMediaClientListener {
    func remoteMediaClient(_ client: GCKRemoteMediaClient, didUpdate mediaStatus: GCKMediaStatus?) {
        mediaChanged(mediaStatus)
    }
}

/// Kivo's namespace on the session: the receiver's `kivo.status` in, Kivo's messages out.
private final class KivoChannel: GCKCastChannel {
    var received: (String) -> Void = { _ in }

    override func didReceiveTextMessage(_ message: String) {
        received(message)
    }
}
