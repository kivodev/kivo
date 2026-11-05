import BackgroundTasks
import KivoCore

/// The downloads' background refresh: while the app isn't open, iOS wakes
/// it now and then, and the core starts what the server finished
/// preparing on the background transfers. It's asked for whenever the app
/// leaves the foreground with a download in line or converting there.
enum DownloadRefresh {
    /// As Info.plist permits it (project.yml).
    static let identifier = "com.kivo.player.downloads.refresh"

    /// Asks iOS for a refresh in a quarter of an hour or so, in place of any asked for before.
    static func schedule() {
        let request = BGAppRefreshTaskRequest(identifier: identifier)
        request.earliestBeginDate = Date(timeIntervalSinceNow: 15 * 60)
        try? BGTaskScheduler.shared.submit(request)
    }

    /// Whether [items] have a download the server hasn't finished preparing.
    static func pending(_ items: [DownloadItem]) -> Bool {
        items.contains { $0.status is DownloadStatusQueued || $0.status is DownloadStatusConverting }
    }
}
