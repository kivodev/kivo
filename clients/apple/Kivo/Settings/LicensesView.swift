import SwiftUI

/// Open source licenses, from Credits: a row for each library, with its version and license, opening its text.
struct LicensesView: View {
    @State private var credits = OpenSourceCredits.load(app: "apple")

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
