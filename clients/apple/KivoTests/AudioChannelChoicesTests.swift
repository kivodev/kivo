import XCTest
@testable import Kivo

/// Maximum audio channels offers what Android TV offers, in its order and
/// with its stored values, so the shared setting reads the same in each.
final class AudioChannelChoicesTests: XCTestCase {
    func testTheChoicesRunAutoStereoFiveOneSevenOne() {
        XCTAssertEqual(AudioSettingsView.channelChoices.map(\.label), ["Auto", "Stereo", "5.1", "7.1"])
        XCTAssertEqual(AudioSettingsView.channelChoices.map(\.channels), [0, 2, 6, 8])
    }
}
