import Foundation
import GoogleCast
import KivoCore

/// Google Cast: the shared context set up with Kivo's receiver, its session
/// manager and the session's media client, and Kivo's namespace, as the
/// core's cast remote drives it.
final class GoogleCastLink: NSObject, KivoCore.CastLink {
    private var events: (any CastLinkEvents)?
    private var channel: KivoChannel?
    private var client: GCKRemoteMediaClient?

    /// Sets up the shared context once, at launch: devices are looked for
    /// from the start, since sign-in already asked for the local network,
    /// and the hardware volume buttons drive the TV.
    static func configure() {
        let options = GCKCastOptions(discoveryCriteria: GCKDiscoveryCriteria(applicationID: CAST_APP_ID))
        options.startDiscoveryAfterFirstTapOnCastButton = false
        options.physicalVolumeButtonsWillControlDeviceVolume = true
        GCKCastContext.setSharedInstanceWith(options)
    }

    /// The system's list of Cast devices to connect to.
    static func showDevices() {
        GCKCastContext.sharedInstance().presentCastDialog()
    }

    var positionMs: Int64 { Int64((client?.approximateStreamPosition() ?? 0) * 1000) }

    func start(events: any CastLinkEvents) {
        self.events = events
        let context = GCKCastContext.sharedInstance()
        context.sessionManager.add(self)
        NotificationCenter.default.addObserver(self, selector: #selector(castStateChanged), name: .gckCastStateDidChange, object: context)
        if let session = context.sessionManager.currentCastSession {
            attach(session, resumed: true)
        } else {
            castStateChanged()
        }
    }

    func load(load: CastLoad, title: String, imageUrl: String?) {
        guard let client else { return }
        let metadata = GCKMediaMetadata(metadataType: .movie)
        metadata.setString(title, forKey: kGCKMetadataKeyTitle)
        if let image = imageUrl.flatMap(URL.init(string:)) { metadata.addImage(GCKImage(url: image, width: 480, height: 720)) }
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

    func seek(positionMs: Int64) {
        let options = GCKMediaSeekOptions()
        options.interval = Double(positionMs) / 1000
        client?.seek(with: options)
    }

    func setSpeed(speed: Float) { client?.setPlaybackRate(speed) }

    func setVolume(volume: Float) {
        GCKCastContext.sharedInstance().sessionManager.currentCastSession?.setDeviceVolume(volume)
    }

    func send(message: String) {
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
        events = nil
    }

    /// Found devices and connecting; a connected session is reported as it starts or resumes.
    @objc private func castStateChanged() {
        switch GCKCastContext.sharedInstance().castState {
        case .noDevicesAvailable: events?.onLink(state: CastLinkStateUnavailable.shared)
        case .notConnected: events?.onLink(state: CastLinkStateDisconnected.shared)
        case .connecting: events?.onLink(state: CastLinkStateConnecting.shared)
        case .connected: break
        @unknown default: events?.onLink(state: CastLinkStateDisconnected.shared)
        }
    }

    private func attach(_ session: GCKCastSession, resumed: Bool) {
        client?.remove(self)
        let channel = KivoChannel(namespace: CAST_NAMESPACE)
        channel.received = { [weak self] in self?.events?.onMessage(message: $0) }
        session.add(channel)
        self.channel = channel
        client = session.remoteMediaClient
        client?.add(self)
        events?.onVolume(volume: session.currentDeviceVolume)
        events?.onLink(state: CastLinkStateConnected(device: session.device.friendlyName ?? "", resumed: resumed))
        mediaChanged(client?.mediaStatus)
    }

    private func detach() {
        client?.remove(self)
        client = nil
        channel = nil
        events?.onMedia(media: nil)
    }

    private func mediaChanged(_ status: GCKMediaStatus?) {
        guard let status, let info = status.mediaInformation, let workId = info.contentID else {
            events?.onMedia(media: nil)
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
                events?.onMedia(media: nil)
                return
            }
            state = .loading
        @unknown default: state = .loading
        }
        let custom = status.customData
            .flatMap { JSONSerialization.isValidJSONObject($0) ? try? JSONSerialization.data(withJSONObject: $0) : nil }
            .map { String(decoding: $0, as: UTF8.self) }
        events?.onMedia(media: CastMedia(
            workId: workId,
            state: state,
            positionMs: Int64(status.streamPosition * 1000),
            durationMs: Int64(max(info.streamDuration, 0) * 1000),
            speed: status.playbackRate,
            status: custom
        ))
    }
}

extension GoogleCastLink: GCKSessionManagerListener {
    func sessionManager(_ sessionManager: GCKSessionManager, didStart session: GCKCastSession) {
        attach(session, resumed: false)
    }

    func sessionManager(_ sessionManager: GCKSessionManager, didResumeCastSession session: GCKCastSession) {
        attach(session, resumed: true)
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
        events?.onVolume(volume: muted ? 0 : volume)
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
