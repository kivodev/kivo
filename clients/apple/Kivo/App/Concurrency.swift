// The core exports the server's `Task` (a background task on the Tasks
// page), which would shadow Swift's own. The app's declaration wins over
// an import's, so `Task` here is always Swift's.
typealias Task = _Concurrency.Task
