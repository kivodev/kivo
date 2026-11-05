import Foundation
import KivoCore

/// The core's DownloadTransport on background `URLSession`s: transfers go
/// on with the app suspended or closed, and iOS relaunches the app to hand
/// over what finished. There's a session for each network rule, one for
/// Wi-Fi only and one that may use cellular, and a request carries no rule
/// of its own, so a transfer moves from one to the other by its resume
/// data when cellular downloads are allowed or no longer are. A transfer
/// that stops keeps what it fetched as resume data, and starting it again
/// resumes it by range, as the server's strong ETag allows, from the URL
/// it's started with: when the server moves to another of its addresses,
/// a transfer under way stops and goes on from there, its resume data
/// pointed at the new URL ([ResumeData]). A host that doesn't answer fails
/// a transfer after [resourceTimeout], so the core starts it again from
/// the address in use. A finished
/// file is moved to its destination before it's reported, so a file at
/// its destination is always whole. Each task carries its transfer's ID
/// and destination, so the app picks up after a relaunch: `prepare`
/// learns what's under way, and events for transfers that finished
/// meanwhile arrive as the sessions are made again.
final class BackgroundTransport: NSObject, DownloadTransport, URLSessionDownloadDelegate, @unchecked Sendable {
    /// The sessions' identifiers, each the app's alone: Wi-Fi only, and cellular allowed.
    static let wifiIdentifier = "com.kivo.player.downloads.wifi"
    static let cellularIdentifier = "com.kivo.player.downloads.cellular"
    static let identifiers: Set<String> = [wifiIdentifier, cellularIdentifier]
    /// How long a transfer may take before it fails, kept what it fetched,
    /// so one stuck on an address that no longer answers doesn't wait for
    /// days, while the longest file still comes in one go on a slow line.
    static let resourceTimeout: TimeInterval = 4 * 60 * 60
    /// The one transport: a background session's identifier is the app's alone.
    static let shared = BackgroundTransport()

    /// Hears each transfer's progress, end or failure; set once the core exists.
    var events: ((TransferEvent) -> Void)?

    private let lock = NSLock()
    /// Each transfer under way, by ID: its task, none while it moves to the other session.
    private var underway: [String: Transfer] = [:]
    /// Tasks stopped here, to move or pause them, whose end isn't the transfer's.
    private var stopping: Set<ObjectIdentifier> = []
    /// Transfers cancelled, whose resume data, arriving late from a move, isn't kept.
    private var cancelled: Set<String> = []
    private var lastProgress: [String: Date] = [:]
    /// iOS's handlers for a background launch, by session, called once its events are delivered.
    private var completions: [String: () -> Void] = [:]
    private var sessions: [String: URLSession] = [:]
    private let resumeFolder: URL

    private struct Transfer {
        var task: URLSessionDownloadTask?
        let request: TransferRequest
    }

    private override init() {
        resumeFolder = URL.cachesDirectory.appending(path: "DownloadResume", directoryHint: .isDirectory)
        try? FileManager.default.createDirectory(at: resumeFolder, withIntermediateDirectories: true)
        super.init()
    }

    /// The session [identifier] names, made the first time it's needed: on
    /// a launch to hand over its events, at `prepare`, or for a transfer.
    private func session(_ identifier: String) -> URLSession {
        lock.withLock {
            if let session = sessions[identifier] { return session }
            let cellular = identifier != Self.wifiIdentifier
            let configuration = URLSessionConfiguration.background(withIdentifier: identifier)
            configuration.isDiscretionary = false
            configuration.sessionSendsLaunchEvents = true
            configuration.allowsCellularAccess = cellular
            configuration.allowsExpensiveNetworkAccess = cellular
            // The core hands over two downloads at a time; this keeps iOS to that too.
            configuration.httpMaximumConnectionsPerHost = 2
            configuration.timeoutIntervalForResource = Self.resourceTimeout
            let session = URLSession(configuration: configuration, delegate: self, delegateQueue: nil)
            sessions[identifier] = session
            return session
        }
    }

    private func session(cellular: Bool) -> URLSession {
        session(cellular ? Self.cellularIdentifier : Self.wifiIdentifier)
    }

    /// Makes a session again, so iOS delivers its events, and keeps iOS's
    /// handler to call once they are: on a launch to hand them over.
    func reconnect(identifier: String, completion: @escaping () -> Void) {
        lock.withLock { completions[identifier] = completion }
        _ = session(identifier)
    }

    /// Reconnects to the sessions and learns which transfers are under
    /// way, before the core asks.
    func prepare() async {
        for cellular in [false, true] {
            for task in await session(cellular: cellular).allTasks where task.state == .running || task.state == .suspended {
                guard let task = task as? URLSessionDownloadTask, let tag = Tag(task.taskDescription), let url = task.originalRequest?.url else { continue }
                let request = TransferRequest(id: tag.id, url: url.absoluteString, authorization: nil, destination: tag.destination, cellular: cellular)
                lock.withLock { underway[tag.id] = Transfer(task: task, request: request) }
            }
        }
    }

    // MARK: DownloadTransport

    func start(request: TransferRequest) {
        enum Step { case none, begin, move(URLSessionDownloadTask) }
        let step: Step = lock.withLock {
            cancelled.remove(request.id)
            guard let current = underway[request.id] else {
                underway[request.id] = Transfer(task: nil, request: request)
                return .begin
            }
            if current.request.cellular == request.cellular, current.request.url == request.url { return .none }
            underway[request.id] = Transfer(task: nil, request: request)
            // A move already on its way begins under this request instead.
            return current.task.map { .move($0) } ?? .none
        }
        switch step {
        case .none:
            break
        case .begin:
            let data = try? Data(contentsOf: resumeFile(request.id))
            try? FileManager.default.removeItem(at: resumeFile(request.id))
            begin(request.id, resuming: data)
        case let .move(task):
            // Under the other rule or from another URL: it stops, and goes on from what it fetched.
            stopped(task)
            task.cancel { [weak self] data in
                self?.begin(request.id, resuming: data)
            }
        }
    }

    func stop(id: String) {
        guard let task = lock.withLock({ underway.removeValue(forKey: id)?.task }) else { return }
        stopped(task)
        task.cancel { [weak self] data in
            guard let self, let data else { return }
            try? data.write(to: self.resumeFile(id))
        }
    }

    func cancel(id: String) {
        let task = lock.withLock {
            cancelled.insert(id)
            return underway.removeValue(forKey: id)?.task
        }
        try? FileManager.default.removeItem(at: resumeFile(id))
        if let task {
            stopped(task)
            task.cancel()
        }
    }

    func running() -> [String] {
        lock.withLock { Array(underway.keys) }
    }

    /// Starts transfer [id]'s task in its rule's session, from [data] when
    /// an earlier attempt left some. One stopped meanwhile keeps [data] for
    /// when it starts again.
    private func begin(_ id: String, resuming data: Data?) {
        let wifi = session(cellular: false), cellular = session(cellular: true)
        let task: URLSessionDownloadTask? = lock.withLock {
            guard let transfer = underway[id], transfer.task == nil else {
                if underway[id] == nil, !cancelled.contains(id), let data { try? data.write(to: resumeFile(id)) }
                return nil
            }
            let request = transfer.request
            let session = request.cellular ? cellular : wifi
            let task: URLSessionDownloadTask
            if let data, let url = URL(string: request.url), let resuming = ResumeData.pointing(data, to: url) {
                task = session.downloadTask(withResumeData: resuming)
            } else {
                guard let url = URL(string: request.url) else { return nil }
                var urlRequest = URLRequest(url: url)
                if let authorization = request.authorization {
                    urlRequest.setValue(authorization, forHTTPHeaderField: "Authorization")
                }
                task = session.downloadTask(with: urlRequest)
            }
            task.taskDescription = Tag(id: id, destination: request.destination).description
            underway[id]?.task = task
            return task
        }
        task?.resume()
    }

    /// A task stopped here, whose end the delegate doesn't report.
    private func stopped(_ task: URLSessionTask) {
        _ = lock.withLock { stopping.insert(ObjectIdentifier(task)) }
    }

    // MARK: URLSessionDownloadDelegate

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData bytesWritten: Int64, totalBytesWritten: Int64, totalBytesExpectedToWrite: Int64) {
        guard let tag = Tag(downloadTask.taskDescription) else { return }
        // A few times a second is plenty for the list.
        let now = Date()
        let due = lock.withLock {
            guard underway[tag.id]?.task === downloadTask else { return false }
            guard now.timeIntervalSince(lastProgress[tag.id] ?? .distantPast) >= 0.25 else { return false }
            lastProgress[tag.id] = now
            return true
        }
        if due {
            events?(TransferEventProgress(id: tag.id, received: totalBytesWritten, expected: totalBytesExpectedToWrite))
        }
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        guard let tag = Tag(downloadTask.taskDescription) else { return }
        let status = (downloadTask.response as? HTTPURLResponse)?.statusCode ?? 0
        // An error's body arrives as a file too: it isn't the file.
        guard (200..<300).contains(status) else { return }
        let destination = URL(fileURLWithPath: tag.destination)
        let files = FileManager.default
        do {
            try files.createDirectory(at: destination.deletingLastPathComponent(), withIntermediateDirectories: true)
            if files.fileExists(atPath: destination.path) { try files.removeItem(at: destination) }
            try files.moveItem(at: location, to: destination)
        } catch {
            NSLog("Kivo: couldn't keep a download's file: \(error)")
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: (any Error)?) {
        guard let tag = Tag(task.taskDescription) else { return }
        let reported = lock.withLock {
            if stopping.remove(ObjectIdentifier(task)) != nil { return false }
            // One that ended while the app was away is reported; one an
            // attempt started since replaces isn't.
            if let current = underway[tag.id] {
                guard current.task === task else { return false }
                underway[tag.id] = nil
            }
            lastProgress[tag.id] = nil
            return true
        }
        guard reported else { return }
        let status = (task.response as? HTTPURLResponse)?.statusCode ?? 0
        if let error = error as NSError? {
            // Kept, so starting it again resumes by range: a dropped network,
            // or the app closed from the app switcher, which cancels it.
            if let data = error.userInfo[NSURLSessionDownloadTaskResumeData] as? Data {
                try? data.write(to: resumeFile(tag.id))
            }
            if error.code == NSURLErrorCancelled, error.userInfo[NSURLSessionDownloadTaskResumeData] == nil {
                return // cancelled by the core
            }
            events?(TransferEventFailed(id: tag.id, status: Int32(status), message: error.localizedDescription))
        } else if (200..<300).contains(status) {
            events?(TransferEventFinished(id: tag.id))
        } else {
            events?(TransferEventFailed(id: tag.id, status: Int32(status), message: HTTPURLResponse.localizedString(forStatusCode: status)))
        }
    }

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        guard let identifier = session.configuration.identifier else { return }
        let completion = lock.withLock { completions.removeValue(forKey: identifier) }
        DispatchQueue.main.async { completion?() }
    }

    private func resumeFile(_ id: String) -> URL {
        resumeFolder.appending(path: id.replacingOccurrences(of: "/", with: "_") + ".resume")
    }
}

/// A task's transfer ID and destination, as its description carries them.
private struct Tag: CustomStringConvertible {
    let id: String
    let destination: String

    init(id: String, destination: String) {
        self.id = id
        self.destination = destination
    }

    init?(_ description: String?) {
        guard let parts = description?.split(separator: "\n", maxSplits: 1), parts.count == 2 else { return nil }
        id = String(parts[0])
        destination = String(parts[1])
    }

    var description: String { "\(id)\n\(destination)" }
}
