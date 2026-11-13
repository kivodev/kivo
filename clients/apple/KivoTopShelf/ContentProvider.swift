import TVServices

/// Top Shelf: Continue Watching, as the app last left it in their app
/// group, each item's still with how far through it the user is. Selecting
/// one opens its page in the app, and Play plays it. With nothing to
/// continue, the app's own Top Shelf image shows.
final class ContentProvider: TVTopShelfContentProvider {
    override func loadTopShelfContent() async -> TVTopShelfContent? {
        let items = TopShelfItems.read().map { item in
            let shelved = TVTopShelfSectionedItem(identifier: item.id)
            shelved.title = item.title
            shelved.imageShape = .hdtv
            if let image = TopShelfItems.image(of: item) {
                shelved.setImageURL(image, for: [.screenScale1x, .screenScale2x])
            }
            if let progress = item.progress { shelved.playbackProgress = progress }
            shelved.displayAction = TVTopShelfAction(url: TopShelfLink.work(item.id).url)
            shelved.playAction = TVTopShelfAction(url: TopShelfLink.play(item.id).url)
            return shelved
        }
        guard !items.isEmpty else { return nil }
        let section = TVTopShelfItemCollection(items: items)
        section.title = String(localized: "Continue Watching")
        return TVTopShelfSectionedContent(sections: [section])
    }
}
