import KivoCore
import KivoUI
import SwiftUI

/// Stream info: the play's method, each track's method with what it was
/// and became, and why for every line that isn't direct; the subtitle
/// showing, the delivery and the bitrate (StreamInfoRow). It's pushed from
/// Playback settings.
struct StreamInfoView: View {
    let model: any PlayerChoices

    var body: some View {
        List(rows) { row in
            VStack(alignment: .leading, spacing: KivoTokens.spacing) {
                LabeledContent(row.label, value: row.value)
                if let why = row.why {
                    Text("Why: \(why)").font(.footnote).foregroundStyle(.secondary)
                }
                if let note = row.note {
                    Text(note).font(.footnote).foregroundStyle(.secondary)
                }
            }
        }
        .navigationTitle(String(localized: "Stream info"))
        .navigationBarTitleDisplayMode(.inline)
    }

    private var rows: [StreamInfoRow] {
        guard let play = model.streamInfo else { return [] }
        return StreamInfoRow.rows(play, display: model.display, version: model.version, subtitle: model.subtitle, offline: model.offline, quality: model.quality)
    }
}
