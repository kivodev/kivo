import Foundation
import Network
import XCTest
@testable import Kivo

/// A transfer's resume data pointed at the server's other address, with
/// real resume data from URLSession: it resumes there by range.
final class ResumeDataTests: XCTestCase {
    private let body = Data((0..<3_000_000).map { UInt8($0 % 251) })

    func testResumeDataPointedAtAnotherAddressResumesThereByRange() async throws {
        let home = try FileServer(body: body, stallAfter: 1_000_000)
        let away = try FileServer(body: body)
        defer {
            home.stop()
            away.stop()
        }
        let homeBase = try await home.start()
        let awayBase = try await away.start()
        let homeFile = try XCTUnwrap(URL(string: homeBase + "/api/v1/downloads/d1/files/0"))
        let awayFile = try XCTUnwrap(URL(string: awayBase + "/api/v1/downloads/d1/files/0"))
        let session = URLSession(configuration: .ephemeral)
        defer { session.invalidateAndCancel() }

        // Part way through at home, the transfer stops, keeping what it fetched.
        var request = URLRequest(url: homeFile)
        request.setValue("Bearer tok", forHTTPHeaderField: "Authorization")
        let task = session.downloadTask(with: request)
        task.resume()
        for _ in 0..<200 where task.countOfBytesReceived < 1_000_000 {
            try await Task.sleep(for: .milliseconds(50))
        }
        XCTAssertEqual(task.countOfBytesReceived, 1_000_000)
        let stopped = await task.cancelByProducingResumeData()
        let data = try XCTUnwrap(stopped, "the home server's answer can be resumed")
        XCTAssertEqual(ResumeData.url(of: data), homeFile)

        let pointed = try XCTUnwrap(ResumeData.pointing(data, to: awayFile))

        XCTAssertEqual(ResumeData.url(of: pointed), awayFile)
        let requests = ResumeData.requests(of: pointed)
        XCTAssertEqual(requests.count, 2, "the original and current requests")
        XCTAssertEqual(requests.map(\.url), [awayFile, awayFile])
        XCTAssertEqual(requests.map { $0.value(forHTTPHeaderField: "Authorization") }, ["Bearer tok", "Bearer tok"])
        XCTAssertEqual(ResumeData.pointing(pointed, to: awayFile), pointed, "already there, it's as it was")

        let (location, response) = try await session.download(resumeFrom: pointed)
        XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 206)
        XCTAssertEqual(try Data(contentsOf: location), body)
        XCTAssertEqual(away.ranges, ["bytes=1000000-"], "the rest only, from the new address")
        XCTAssertEqual(home.ranges, [nil])
    }

    func testResumeDataItCantReadStartsOver() throws {
        let url = try XCTUnwrap(URL(string: "http://192.168.1.20:8484/api/v1/downloads/d1/files/0"))
        XCTAssertNil(ResumeData.pointing(Data("not resume data".utf8), to: url))
        let plist = try PropertyListSerialization.data(fromPropertyList: ["NSURLSessionDownloadURL": "http://elsewhere/x"], format: .binary, options: 0)
        XCTAssertNil(ResumeData.pointing(plist, to: url), "without its requests")
    }
}

/// A file on localhost served by range with a strong ETag, as the server
/// serves a download's files; with [stallAfter], a whole-file answer stops
/// sending after that many bytes and holds the connection. It keeps each
/// request's Range header.
private final class FileServer: @unchecked Sendable {
    private let body: Data
    private let stallAfter: Int?
    private let listener: NWListener
    private let queue = DispatchQueue(label: "file-server")
    private let lock = NSLock()
    private var seen: [String?] = []
    private var connections: [NWConnection] = []

    init(body: Data, stallAfter: Int? = nil) throws {
        self.body = body
        self.stallAfter = stallAfter
        listener = try NWListener(using: .tcp, on: .any)
        listener.newConnectionHandler = { [weak self] connection in self?.serve(connection) }
    }

    var ranges: [String?] {
        lock.withLock { seen }
    }

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
        lock.withLock { connections.forEach { $0.cancel() } }
    }

    private func serve(_ connection: NWConnection) {
        lock.withLock { connections.append(connection) }
        connection.start(queue: queue)
        read(connection, Data())
    }

    private func read(_ connection: NWConnection, _ buffer: Data) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: 65536) { [weak self] data, _, done, error in
            guard let self else { return }
            var buffer = buffer
            if let data { buffer.append(data) }
            guard let text = String(data: buffer, encoding: .utf8), text.contains("\r\n\r\n") else {
                if done || error != nil { connection.cancel() } else { self.read(connection, buffer) }
                return
            }
            let range = text.components(separatedBy: "\r\n")
                .first { $0.lowercased().hasPrefix("range:") }
                .map { $0.dropFirst("range:".count).trimmingCharacters(in: .whitespaces) }
            self.lock.withLock { self.seen.append(range) }
            self.answer(connection, range: range)
        }
    }

    private func answer(_ connection: NWConnection, range: String?) {
        let total = body.count
        var head = "Accept-Ranges: bytes\r\nETag: \"files-0\"\r\nLast-Modified: Mon, 19 Oct 2026 12:00:00 GMT\r\nContent-Type: video/mp4\r\n"
        let from = range.flatMap { Int($0.replacingOccurrences(of: "bytes=", with: "").split(separator: "-").first ?? "") } ?? 0
        if range != nil {
            head = "HTTP/1.1 206 Partial Content\r\n" + head + "Content-Range: bytes \(from)-\(total - 1)/\(total)\r\nContent-Length: \(total - from)\r\n\r\n"
        } else {
            head = "HTTP/1.1 200 OK\r\n" + head + "Content-Length: \(total)\r\n\r\n"
        }
        let sent = range == nil ? stallAfter.map { body.prefix($0) } ?? body : body.suffix(from: from)
        connection.send(content: Data(head.utf8) + sent, completion: .contentProcessed { _ in })
    }
}
