import Foundation
import KivoCore

/// What the builder and a smart list's page call kinds, fields, operators and
/// values. A field's label is by its key, or the catalog's name for one this
/// app doesn't know.
enum SmartListWords {
    static func kind(_ kind: WorkKind) -> String {
        switch kind {
        case .movie: String(localized: "Movies")
        case .series: String(localized: "Shows")
        case .season: String(localized: "Seasons")
        case .episode: String(localized: "Episodes")
        case .video: String(localized: "Videos")
        case .book: String(localized: "Books")
        case .extra, .unknown: kind.value
        }
    }

    /// "42 movies", or "42 titles" over more than one kind.
    static func count(_ count: Int, kinds: [WorkKind]) -> String {
        let n = count.formatted()
        let one = count == 1
        switch kinds.count == 1 ? kinds[0] : nil {
        case .movie: return one ? String(localized: "1 movie") : String(localized: "\(n) movies")
        case .series: return one ? String(localized: "1 show") : String(localized: "\(n) shows")
        case .season: return one ? String(localized: "1 season") : String(localized: "\(n) seasons")
        case .episode: return one ? String(localized: "1 episode") : String(localized: "\(n) episodes")
        case .video: return one ? String(localized: "1 video") : String(localized: "\(n) videos")
        case .book: return one ? String(localized: "1 book") : String(localized: "\(n) books")
        case .extra, .unknown, nil: return one ? String(localized: "1 title") : String(localized: "\(n) titles")
        }
    }

    static func field(_ field: WorkField) -> String {
        switch field.field {
        case "library": String(localized: "Library")
        case "contentRating": String(localized: "Content rating")
        case "playState": String(localized: "Play state")
        case "genre": String(localized: "Genre")
        case "tag": String(localized: "Tag")
        case "company": String(localized: "Company")
        case "resolution": String(localized: "Resolution")
        case "dynamicRange": String(localized: "Dynamic range")
        case "collection": String(localized: "Collection")
        case "bookSeries": String(localized: "Book series")
        case "inList": String(localized: "In list")
        case "rootWork": String(localized: "Show or movie")
        case "actor": String(localized: "Actor")
        case "director": String(localized: "Director")
        case "author": String(localized: "Author")
        case "narrator": String(localized: "Narrator")
        case "rating": String(localized: "Rating")
        case "myRating": String(localized: "My rating")
        case "runtimeMs": String(localized: "Runtime")
        case "year": String(localized: "Year")
        case "addedAt": String(localized: "Date added")
        case "lastEpisodeAddedAt": String(localized: "Latest addition")
        case "releaseDate": String(localized: "Release date")
        case "lastPlayedAt": String(localized: "Last played")
        case "title": String(localized: "Title")
        default: field.name
        }
    }

    static func op(_ op: String, type: WorkFieldType) -> String {
        switch op {
        case "anyOf", "eq": String(localized: "is")
        case "allOf": String(localized: "is all of")
        case "noneOf": String(localized: "is not")
        case "isSet": String(localized: "is set")
        case "isNotSet": String(localized: "isn't set")
        case "gte": type == .date ? String(localized: "on or after") : String(localized: "at least")
        case "lte": type == .date ? String(localized: "on or before") : String(localized: "at most")
        case "between": String(localized: "between")
        case "within": String(localized: "in the last")
        case "notWithin": String(localized: "not in the last")
        case "contains": String(localized: "contains")
        case "notContains": String(localized: "doesn't contain")
        case "startsWith": String(localized: "starts with")
        default: op
        }
    }

    /// A sort as the Sort row names it: "Rating, highest first".
    static func sort(_ field: WorkField, direction: SortDirection) -> String {
        let order = switch (field.type, direction) {
        case (.date, .asc): String(localized: "oldest first")
        case (.date, _): String(localized: "newest first")
        case (.text, .desc): String(localized: "Z to A")
        case (.text, _): String(localized: "A to Z")
        case (_, .asc): field.unit == "ms" ? String(localized: "shortest first") : String(localized: "lowest first")
        default: field.unit == "ms" ? String(localized: "longest first") : String(localized: "highest first")
        }
        return "\(Self.field(field)), \(order)"
    }

    /// A period value, "P30D", as words: "30 days".
    static func period(_ value: String) -> String {
        let amount = Int(SmartLists.shared.periodAmount(value: value)) ?? 0
        let one = amount == 1
        return switch SmartLists.shared.periodUnit(value: value) {
        case "D": one ? String(localized: "1 day") : String(localized: "\(amount) days")
        case "W": one ? String(localized: "1 week") : String(localized: "\(amount) weeks")
        case "M": one ? String(localized: "1 month") : String(localized: "\(amount) months")
        default: one ? String(localized: "1 year") : String(localized: "\(amount) years")
        }
    }

    /// A value in milliseconds as minutes.
    static func minutes(_ value: String) -> String {
        guard let ms = Double(value) else { return value }
        return (ms / 60000).formatted(.number.grouping(.never))
    }

    /// A rule's values to read: names where the values are chosen, joined by
    /// "or" ("and" when all must match), a period in words, a duration in
    /// minutes, the two ends of a range joined by [to].
    static func values(_ rule: RuleDraft, of field: WorkField, names: [String: String], to: String = String(localized: "and")) -> String {
        if SmartLists.shared.isPeriod(op: rule.op) {
            return rule.values.first.map(period) ?? ""
        }
        if rule.arity == -1 {
            let separator = rule.op == "allOf" ? String(localized: " and ") : String(localized: " or ")
            return rule.values.map { names[SmartLists.shared.nameKey(field: rule.field, value: $0)] ?? $0 }.joined(separator: separator)
        }
        let shown = field.unit == "ms" ? rule.values.map { String(localized: "\(minutes($0)) min") } : rule.values
        return shown.joined(separator: " \(to) ")
    }

    /// [query]'s rules in a line: "Comedy or Drama, no Animation, 1990 to 1999, (Unplayed or Runtime at most 90 min)".
    static func summary(of query: WorkQuery, fields: [WorkField], names: [String: String]) -> String {
        guard let root = query.where else { return "" }
        return phrases(of: root, fields: fields, names: names).joined(separator: ", ")
    }

    private static func phrases(of group: RuleGroup, fields: [WorkField], names: [String: String]) -> [String] {
        let rules = (group.rules ?? []).filter { $0.field != "library" }.compactMap { rule -> String? in
            fields.first { $0.field == rule.field }.map { phrase(rule, of: $0, names: names) }
        }
        let groups = (group.groups ?? []).map { group -> String in
            let inner = phrases(of: group, fields: fields, names: names)
                .joined(separator: group.match == .all ? String(localized: " and ") : String(localized: " or "))
            return "(\(inner))"
        }
        return rules + groups
    }

    private static func phrase(_ rule: Rule, of field: WorkField, names: [String: String]) -> String {
        let draft = SmartLists.shared.ruleDraft(rule: rule, fields: [field])
        let label = Self.field(field)
        switch rule.op {
        case "anyOf", "allOf": return values(draft, of: field, names: names)
        case "noneOf": return String(localized: "no \(values(draft, of: field, names: names))")
        case "isSet", "isNotSet": return "\(label) \(op(rule.op, type: field.type))"
        case "between":
            let range = values(draft, of: field, names: names, to: String(localized: "to"))
            return field.field == "year" ? range : "\(label) \(range)"
        default:
            return "\(label) \(op(rule.op, type: field.type)) \(values(draft, of: field, names: names))"
        }
    }
}
