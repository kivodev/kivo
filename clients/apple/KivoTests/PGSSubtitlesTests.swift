import CoreGraphics
import XCTest
@testable import Kivo

/// A PGS file's compositions, from Fixtures/pgs-sample.sup
/// (clients/scripts/gen-pgs-sample.py): a line and its clear in one epoch,
/// then in the next a line, a sign added above it, the line left alone from
/// the epoch's objects, and the clear.
final class PGSSubtitlesTests: XCTestCase {
    private func fixture() throws -> PGSSubtitles {
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "pgs-sample", withExtension: "sup"))
        return try PGSSubtitles(data: Data(contentsOf: url))
    }

    func testTheCompositionsAndWhereTheirObjectsGo() throws {
        let subtitles = try fixture()
        XCTAssertEqual(subtitles.width, 1920)
        XCTAssertEqual(subtitles.height, 1080)
        XCTAssertEqual(subtitles.compositions.map(\.startMs), [14100, 15700, 25200, 27000, 28600, 30000])
        XCTAssertEqual(subtitles.compositions.map(\.objects.count), [1, 0, 1, 2, 1, 0])

        let line = try XCTUnwrap(subtitles.compositions[0].objects.first)
        XCTAssertEqual([line.x, line.y, line.picture.width, line.picture.height], [743, 950, 434, 50])
        XCTAssertNil(line.crop)
        XCTAssertFalse(line.forced)

        let both = subtitles.compositions[3].objects
        XCTAssertEqual(both.map { [$0.x, $0.y, $0.picture.width, $0.picture.height] }, [[725, 950, 470, 50], [817, 100, 286, 94]])
        // Left from the epoch's objects, without being sent again.
        let kept = try XCTUnwrap(subtitles.compositions[4].objects.first)
        XCTAssertEqual([kept.x, kept.y, kept.picture.width, kept.picture.height], [725, 950, 470, 50])
    }

    func testTheCompositionShowingAtATime() throws {
        let subtitles = try fixture()
        XCTAssertNil(subtitles.composition(atMs: 0))
        XCTAssertEqual(subtitles.composition(atMs: 14100), 0)
        XCTAssertEqual(subtitles.composition(atMs: 15699), 0)
        XCTAssertEqual(subtitles.composition(atMs: 20000), 1)
        XCTAssertEqual(subtitles.composition(atMs: 27500), 3)
        XCTAssertEqual(subtitles.composition(atMs: 3_600_000), 5)
    }

    func testThePicturesDecodeToTheirRunsInTheirPalette() throws {
        let subtitles = try fixture()
        // As the generator drew them: each line ends where the picture's width does.
        let opaque = { (picture: PGSSubtitles.Picture) in picture.pixels().count { $0 >> 24 != 0 } }
        XCTAssertEqual(opaque(subtitles.compositions[0].objects[0].picture), 13828)
        XCTAssertEqual(opaque(subtitles.compositions[3].objects[0].picture), 16076)
        XCTAssertEqual(opaque(subtitles.compositions[3].objects[1].picture), 19120)
        let image = try XCTUnwrap(subtitles.compositions[3].objects[1].picture.image())
        XCTAssertEqual([image.width, image.height], [286, 94])
    }

    func testPaletteColorsArePremultipliedRGBA() {
        // Opaque white, limited-range black and transparent.
        XCTAssertEqual(PGSSubtitles.rgba(y: 235, cr: 128, cb: 128, alpha: 255), 0xFFFF_FFFF)
        XCTAssertEqual(PGSSubtitles.rgba(y: 16, cr: 128, cb: 128, alpha: 255), 0xFF00_0000)
        XCTAssertEqual(PGSSubtitles.rgba(y: 235, cr: 128, cb: 128, alpha: 0), 0)
    }

    func testAMalformedFileThrows() {
        XCTAssertThrowsError(try PGSSubtitles(data: Data("not a sup file".utf8)))
    }

    func testThePictureShowingPlacesItsObjectsOnTheCanvas() throws {
        let renderer = try PGSRenderer(fixture())
        let canvas = SubtitleCanvas(size: CGSize(width: 960, height: 540), scale: 2, video: CGSize(width: 1920, height: 1080))
        let drawn = try XCTUnwrap(renderer.render(ms: 27500, canvas: canvas))
        XCTAssertEqual(drawn.map(\.frame), [CGRect(x: 362.5, y: 475, width: 235, height: 25), CGRect(x: 408.5, y: 50, width: 143, height: 47)])
        XCTAssertNil(renderer.render(ms: 28000, canvas: canvas), "the same composition draws nothing new")
        XCTAssertEqual(renderer.render(ms: 30500, canvas: canvas)?.count, 0)
    }
}
