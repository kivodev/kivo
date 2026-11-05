import KivoCore
import KivoUI
import SwiftUI

/// About: the app's and the server's versions.
struct AboutView: View {
    @Environment(ServerModel.self) private var server
    @State private var serverVersion: String?
    @State private var failure: String?

    /// The version and, after it, the build, so two builds of one version can be told apart.
    private var appVersion: String {
        let info = Bundle.main.infoDictionary ?? [:]
        let version = info["CFBundleShortVersionString"] as? String ?? ""
        guard let build = info["CFBundleVersion"] as? String, !build.isEmpty else { return version }
        return "\(version) (\(build))"
    }

    var body: some View {
        Form {
            Section {
                LabeledContent(String(localized: "App version"), value: appVersion)
                if let failure {
                    FailureRow(message: failure) { Task { await load() } }
                } else {
                    LabeledContent(String(localized: "Server version"), value: serverVersion ?? "")
                }
            }
        }
        .navigationTitle(String(localized: "About"))
        .toolbarTitleDisplayMode(.inline)
        .task { await load() }
    }

    private func load() async {
        do {
            serverVersion = try await server.session.serverInfo().version
            failure = nil
        } catch {
            failure = error.problemSentence
        }
    }
}
