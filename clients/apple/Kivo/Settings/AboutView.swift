import KivoCore
import KivoUI
import SwiftUI

/// About: the app's and the server's versions, with the credits under them.
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
            } footer: {
                credits
            }
        }
        .navigationTitle(String(localized: "About"))
        .toolbarTitleDisplayMode(.inline)
        .task { await load() }
    }

    /// Where the metadata and the transcoder come from: TMDB's logo and the notice its API terms ask for, then Audnexus and jellyfin-ffmpeg.
    private var credits: some View {
        VStack(alignment: .leading, spacing: 6) {
            Image("TMDBLogo")
                .resizable()
                .scaledToFit()
                .frame(height: 10)
                .accessibilityLabel(Text(verbatim: "TMDB"))
                .padding(.bottom, 2)
            Text(String(localized: "This product uses the TMDB API but is not endorsed or certified by TMDB."))
            Text(String(localized: "Audiobook metadata from Audnexus"))
            Text(String(localized: "Transcoding by jellyfin-ffmpeg (GPL)"))
        }
        .padding(.top, 12)
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
