import Foundation
import SwiftUI

/// A library in the app, or terms no build reports, with its license and that license's text.
struct OpenSourceCredit: Identifiable, Hashable {
    let id: String
    let name: String
    let version: String?
    let license: String?
    let url: URL?
    let text: String?
}

/// The app's open source licenses, from what its build bundles: the Swift
/// packages (Licenses.plist, scripts/licenses.sh), the Kotlin core's
/// libraries (KivoCore-licenses.json, scripts/build-core.sh), then the
/// entries of shared/credits/terms.json that name the iPhone.
enum OpenSourceCredits {
    static func load(bundle: Bundle = .main) -> [OpenSourceCredit] {
        let libraries = (packages(bundle) + core(bundle))
            .sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        return libraries + terms(bundle)
    }

    /// LicensePlist's single page: a group per package, its title the name and version, its footer the license.
    private static func packages(_ bundle: Bundle) -> [OpenSourceCredit] {
        guard let url = bundle.url(forResource: "Licenses", withExtension: "plist"),
              let plist = NSDictionary(contentsOf: url),
              let specifiers = plist["PreferenceSpecifiers"] as? [[String: Any]]
        else { return [] }
        return specifiers.compactMap { item in
            guard let title = item["Title"] as? String, let text = item["FooterText"] as? String else { return nil }
            // "Nuke (13.2.0)", with --add-version-numbers.
            var name = title
            var version: String?
            if let open = title.lastIndex(of: "("), title.hasSuffix(")") {
                name = title[..<open].trimmingCharacters(in: .whitespaces)
                version = String(title[title.index(after: open)..<title.index(before: title.endIndex)])
            }
            return OpenSourceCredit(id: "package:\(name)", name: name, version: version, license: nil, url: nil, text: text)
        }
    }

    private struct AboutLibraries: Decodable {
        struct Library: Decodable {
            let uniqueId: String
            let artifactVersion: String?
            let name: String
            let licenses: [String]
            let targets: [String]?
        }

        struct License: Decodable {
            let name: String
            let spdxId: String?
            let content: String?
        }

        let libraries: [Library]
        let licenses: [String: License]
    }

    /// The core's libraries that its Apple build links: AboutLibraries reports those of the shared and Apple
    /// source sets as the metadata target, and leaves out the Android and JVM ones.
    private static func core(_ bundle: Bundle) -> [OpenSourceCredit] {
        guard let url = bundle.url(forResource: "KivoCore-licenses", withExtension: "json"),
              let data = try? Data(contentsOf: url),
              let file = try? JSONDecoder().decode(AboutLibraries.self, from: data)
        else { return [] }
        return file.libraries.filter { $0.targets?.contains("metadata") ?? true }.map { library in
            let licenses = library.licenses.compactMap { file.licenses[$0] }
            let text = licenses.compactMap(\.content).joined(separator: "\n\n")
            return OpenSourceCredit(
                id: library.uniqueId,
                name: library.name,
                version: library.artifactVersion,
                license: licenses.map { $0.spdxId ?? $0.name }.joined(separator: " AND "),
                url: nil,
                text: text.isEmpty ? nil : text
            )
        }
    }

    private struct Terms: Decodable {
        struct Entry: Decodable {
            let name: String
            let license: String
            let url: String?
            let text: String?
            let apps: [String]
        }

        let terms: [Entry]
    }

    private static func terms(_ bundle: Bundle) -> [OpenSourceCredit] {
        guard let url = bundle.url(forResource: "terms", withExtension: "json", subdirectory: "credits"),
              let data = try? Data(contentsOf: url),
              let file = try? JSONDecoder().decode(Terms.self, from: data)
        else { return [] }
        return file.terms.filter { $0.apps.contains("apple") }.map { entry in
            let text = entry.text.flatMap { name in
                bundle.url(forResource: name, withExtension: nil, subdirectory: "credits")
                    .flatMap { try? String(contentsOf: $0, encoding: .utf8) }
            }
            return OpenSourceCredit(
                id: "terms:\(entry.name)",
                name: entry.name,
                version: nil,
                license: entry.license,
                url: entry.url.flatMap(URL.init(string:)),
                text: text
            )
        }
    }
}

/// Open source licenses, from Credits: a row for each library, with its version and license, opening its text.
struct LicensesView: View {
    @State private var credits = OpenSourceCredits.load()

    var body: some View {
        List(credits) { credit in
            if credit.text != nil {
                NavigationLink { LicenseTextView(credit: credit) } label: { CreditLabel(credit: credit) }
            } else if let url = credit.url {
                Link(destination: url) { CreditLabel(credit: credit) }
                    .foregroundStyle(.primary)
            } else {
                CreditLabel(credit: credit)
            }
        }
        .navigationTitle(String(localized: "Open source licenses"))
        .toolbarTitleDisplayMode(.inline)
    }
}

private struct CreditLabel: View {
    let credit: OpenSourceCredit

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(verbatim: credit.name)
            let detail = [credit.version, credit.license].compactMap(\.self).joined(separator: " · ")
            if !detail.isEmpty {
                Text(verbatim: detail)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }
}

/// One license's text.
private struct LicenseTextView: View {
    let credit: OpenSourceCredit

    var body: some View {
        ScrollView {
            Text(verbatim: credit.text ?? "")
                .font(.footnote.monospaced())
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding()
        }
        .navigationTitle(Text(verbatim: credit.name))
        .toolbarTitleDisplayMode(.inline)
    }
}
