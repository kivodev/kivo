import Foundation
import Network

/// A server on localhost for a test: each request's method, path, query,
/// headers and body go to [answer], whose status and JSON body are sent
/// back, after [delay]. It keeps every request it saw.
final class StubServer: @unchecked Sendable {
    struct Request {
        var method = "GET"
        let path: String
        let query: [String: String]
        /// By lowercased name.
        var headers: [String: String] = [:]
        var body = Data()
    }

    private let listener: NWListener
    private let queue = DispatchQueue(label: "stub-server")
    private let lock = NSLock()
    private var seen: [Request] = []
    private let answer: @Sendable (Request) -> (Int, String)
    var delay: TimeInterval = 0

    init(answer: @escaping @Sendable (Request) -> (Int, String)) throws {
        self.answer = answer
        listener = try NWListener(using: .tcp, on: .any)
        listener.newConnectionHandler = { [weak self] connection in self?.serve(connection) }
    }

    /// Starts listening; the address to reach it at.
    func start() async throws -> String {
        let ready = AsyncStream<NWEndpoint.Port?> { continuation in
            listener.stateUpdateHandler = { [listener] state in
                switch state {
                case .ready: continuation.yield(listener.port)
                case .failed: continuation.yield(nil)
                default: break
                }
            }
        }
        listener.start(queue: queue)
        for await port in ready {
            guard let port else { break }
            return "http://127.0.0.1:\(port.rawValue)"
        }
        throw URLError(.cannotConnectToHost)
    }

    func stop() {
        listener.cancel()
    }

    /// The requests seen to [path].
    func requests(to path: String) -> [Request] {
        lock.lock()
        defer { lock.unlock() }
        return seen.filter { $0.path == path }
    }

    private func serve(_ connection: NWConnection) {
        connection.start(queue: queue)
        read(connection, Data())
    }

    private func read(_ connection: NWConnection, _ buffer: Data) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: 65536) { [weak self] data, _, done, error in
            guard let self else { return }
            var buffer = buffer
            if let data { buffer.append(data) }
            let end = buffer.range(of: Data("\r\n\r\n".utf8))
            let lines = (end.map { String(decoding: buffer[..<$0.lowerBound], as: UTF8.self) } ?? "").split(separator: "\r\n")
            var headers: [String: String] = [:]
            for line in lines.dropFirst() {
                guard let colon = line.firstIndex(of: ":") else { continue }
                headers[line[..<colon].lowercased()] = line[line.index(after: colon)...].trimmingCharacters(in: .whitespaces)
            }
            let sent = end.map { buffer[$0.upperBound...] } ?? Data()
            guard end != nil, sent.count >= Int(headers["content-length"] ?? "0") ?? 0 else {
                if done || error != nil { connection.cancel() } else { self.read(connection, buffer) }
                return
            }
            let start = lines.first?.split(separator: " ") ?? []
            let target = start.dropFirst().first.map(String.init) ?? "/"
            let parts = URLComponents(string: target)
            var query: [String: String] = [:]
            for item in parts?.queryItems ?? [] { query[item.name] = item.value ?? "" }
            let request = Request(
                method: start.first.map(String.init) ?? "GET",
                path: parts?.path ?? target,
                query: query,
                headers: headers,
                body: Data(sent)
            )
            self.lock.lock()
            self.seen.append(request)
            self.lock.unlock()
            let (status, body) = self.answer(request)
            let bytes = Data(body.utf8)
            let type = status < 300 ? "application/json" : "application/problem+json"
            let head = "HTTP/1.1 \(status) X\r\nContent-Type: \(type)\r\nContent-Length: \(bytes.count)\r\nConnection: close\r\n\r\n"
            self.queue.asyncAfter(deadline: .now() + self.delay) {
                connection.send(content: Data(head.utf8) + bytes, completion: .contentProcessed { _ in connection.cancel() })
            }
        }
    }
}
