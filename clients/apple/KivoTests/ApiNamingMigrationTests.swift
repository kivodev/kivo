import XCTest
@testable import Kivo

// Migration (api-naming): remove after every client has updated past this release.
/// A grid's kept sort and filters from an earlier version read in today's names.
final class ApiNamingMigrationTests: XCTestCase {
    // Migration (api-naming): remove after every client has updated past this release.
    func testAKeptGridStateTakesTheSortAndFilterNamesOfNow() {
        let migrated = ApiNamingMigration.gridState([
            "tab": "titles", "sort": "lastEpisodeAdded", "favorites": "false", "resolutions": "4k", "hdr": "dolbyVision,hdr",
        ])
        XCTAssertEqual(migrated, [
            "tab": "titles", "sort": "lastEpisodeAddedAt", "favorites": "false", "resolutions": "4k", "dynamicRanges": "dolbyVision,hdr",
        ])
    }

    // Migration (api-naming): remove after every client has updated past this release.
    func testAGridStateInTodaysNamesIsLeftAlone() {
        XCTAssertNil(ApiNamingMigration.gridState(["sort": "lastEpisodeAddedAt", "dynamicRanges": "hdr"]))
        XCTAssertNil(ApiNamingMigration.gridState([:]))
    }
}
