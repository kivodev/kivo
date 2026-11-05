import Foundation

// Migration (api-naming): remove after every client has updated past this release.
/// Reads what an earlier version of the app kept on the device under the
/// API's earlier names, in the names it uses now.
enum ApiNamingMigration {
    // Migration (api-naming): remove after every client has updated past this release.
    /// A library grid's kept sort and filters, as `GridModel` keeps them:
    /// the sort `lastEpisodeAdded` is `lastEpisodeAddedAt`, and the `hdr`
    /// filter is `dynamicRanges`. Nil when there's nothing to change.
    static func gridState(_ saved: [String: String]) -> [String: String]? {
        var state = saved
        if state["sort"] == "lastEpisodeAdded" {
            state["sort"] = "lastEpisodeAddedAt"
        }
        if let hdr = state.removeValue(forKey: "hdr"), state["dynamicRanges"] == nil {
            state["dynamicRanges"] = hdr
        }
        return state == saved ? nil : state
    }
}
