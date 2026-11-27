import KivoCore

extension BookInfo {
    /// The series the book is mainly in: the one marked primary, else the first.
    var primarySeries: BookSeriesRef? {
        series.first(where: \.primary) ?? series.first
    }

    /// The book's position in the series [seriesId] as written, such as "2" or "1.5".
    func position(in seriesId: String) -> String? {
        series.first { $0.id == seriesId }?.position
    }
}

extension BookSeriesRef {
    /// A book page's eyebrow: "The First Law · Book 2", or the name alone without a position.
    var eyebrow: String {
        position.map { String(localized: "\(name) · Book \($0)") } ?? name
    }
}

extension BookSeries {
    /// "Book 2" over a book's title on this page: "" where another book has
    /// a position and this one hasn't, so the cards line up, and nil when
    /// no book has one.
    func overline(of book: WorkSummary) -> String? {
        guard items.contains(where: { $0.book?.position(in: id) != nil }) else { return nil }
        return book.book?.position(in: id).map { String(localized: "Book \($0)") } ?? ""
    }

    /// The primary author more than half the books share, whose name the
    /// cards leave out; nil when no author does.
    var commonAuthorId: String? {
        let ids = items.compactMap { $0.book?.authors.first?.id }
        let counts = Dictionary(ids.map { ($0, 1) }, uniquingKeysWith: +)
        guard let best = counts.max(by: { $0.value < $1.value }), best.value * 2 > items.count else { return nil }
        return best.key
    }

    /// Whether any card names its authors: some book's primary author isn't the common one.
    var namesAuthors: Bool {
        guard let common = commonAuthorId else { return true }
        return items.contains { $0.book?.authors.first?.id != common }
    }

    /// A card's second line: blank for a book by the common author, so the
    /// cards that name another author line up; nil for the book's own line.
    func authorLine(of book: WorkSummary) -> String? {
        guard let common = commonAuthorId, book.book?.authors.first?.id == common else { return nil }
        return ""
    }
}
