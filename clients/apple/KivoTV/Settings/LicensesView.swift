import SwiftUI

/// Open source licenses, from Credits: a row for each library, with its
/// version and license, opening its text where the app has it.
struct LicensesView: View {
    @State private var credits = OpenSourceCredits.load(app: "appletv")

    var body: some View {
        List(credits) { credit in
            if credit.text != nil {
                NavigationLink { LicenseTextView(credit: credit) } label: { CreditLabel(credit: credit) }
            } else {
                CreditLabel(credit: credit)
                    .focusable()
            }
        }
        .frame(maxWidth: 1300)
        .navigationTitle(String(localized: "Open source licenses"))
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
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }
}

/// One license's text, a paragraph at a time, each taking focus so the remote scrolls through it.
private struct LicenseTextView: View {
    let credit: OpenSourceCredit

    var body: some View {
        let paragraphs = (credit.text ?? "").components(separatedBy: "\n\n").filter { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 24) {
                ForEach(Array(paragraphs.enumerated()), id: \.offset) { _, paragraph in
                    Text(verbatim: paragraph)
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .focusable()
                }
            }
            .frame(maxWidth: 1300)
            .frame(maxWidth: .infinity)
        }
        .navigationTitle(Text(verbatim: credit.name))
    }
}
