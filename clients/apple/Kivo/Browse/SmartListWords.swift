import Foundation
import KivoCore

/// What the builder and a smart list's page call kinds, fields, operators and
/// values. A field's label is by its key, or the catalog's name for one this
/// app doesn't know.
enum SmartListWords {
    /// The sort key that isn't a field.
    static let randomSort = "random"

    static func kind(_ kind: String) -> String {
        switch kind {
        case "movie": String(localized: "Movies")
        case "series": String(localized: "Shows")
        case "season": String(localized: "Seasons")
        case "episode": String(localized: "Episodes")
        case "video": String(localized: "Videos")
        case "book": String(localized: "Books")
        default: kind
        }
    }

    /// "42 movies", or "42 titles" over more than one kind.
    static func count(_ count: Int, kinds: [String]) -> String {
        let n = count.formatted()
        let one = count == 1
        switch kinds.count == 1 ? kinds[0] : "" {
        case "movie": return one ? String(localized: "1 movie") : String(localized: "\(n) movies")
        case "series": return one ? String(localized: "1 show") : String(localized: "\(n) shows")
        case "season": return one ? String(localized: "1 season") : String(localized: "\(n) seasons")
        case "episode": return one ? String(localized: "1 episode") : String(localized: "\(n) episodes")
        case "video": return one ? String(localized: "1 video") : String(localized: "\(n) videos")
        case "book": return one ? String(localized: "1 book") : String(localized: "\(n) books")
        default: return one ? String(localized: "1 title") : String(localized: "\(n) titles")
        }
    }

    /// A list's kinds in a phrase: "Movies and shows".
    static func kinds(_ kinds: [String]) -> String {
        let phrase = ListFormatter.localizedString(byJoining: kinds.map { kind($0).lowercased() })
        return phrase.prefix(1).uppercased() + phrase.dropFirst()
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

    static func op(_ op: String, type: String) -> String {
        switch op {
        case "anyOf", "eq": String(localized: "is")
        case "allOf": String(localized: "is all of")
        case "noneOf": String(localized: "is not")
        case "isSet": String(localized: "is set")
        case "isNotSet": String(localized: "is not set")
        case "gte": type == "date" ? String(localized: "on or after") : String(localized: "at least")
        case "lte": type == "date" ? String(localized: "on or before") : String(localized: "at most")
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
    static func sort(_ field: WorkField, direction: String) -> String {
        let order = switch (field.type, direction) {
        case ("date", "asc"): String(localized: "oldest first")
        case ("date", _): String(localized: "newest first")
        case ("text", "desc"): String(localized: "Z to A")
        case ("text", _): String(localized: "A to Z")
        case (_, "asc"): field.unit == "ms" ? String(localized: "shortest first") : String(localized: "lowest first")
        default: field.unit == "ms" ? String(localized: "longest first") : String(localized: "highest first")
        }
        return "\(Self.field(field)), \(order)"
    }

    /// A period value, "P30D", as words: "30 days".
    static func period(_ value: String) -> String {
        let amount = Int(value.dropFirst().dropLast()) ?? 0
        let one = amount == 1
        return switch value.last {
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

    /// A rule's values to read: names where the values are chosen, a period
    /// in words, a duration in minutes, the two ends of a range joined by [to].
    static func values(_ rule: RuleDraft, of field: WorkField, names: [String: String], to: String = String(localized: "and")) -> String {
        if rule.op == "within" || rule.op == "notWithin" {
            return rule.values.first.map(period) ?? ""
        }
        if rule.arity == -1 {
            return rule.values.map { names[nameKey(rule.field, $0)] ?? $0 }.joined(separator: ", ")
        }
        let shown = field.unit == "ms" ? rule.values.map { String(localized: "\(minutes($0)) min") } : rule.values
        return shown.joined(separator: " \(to) ")
    }

    /// The key of a value in a names map.
    static func nameKey(_ field: String, _ value: String) -> String {
        "\(field)/\(value)"
    }

    /// Reads the names of [query]'s chosen values, as a names map.
    static func names(of query: WorkQuery, fields: [WorkField], session: KivoSession) async -> [String: String] {
        var wanted: [String: Set<String>] = [:]
        func collect(_ group: RuleGroup) {
            for rule in group.rules ?? [] where fields.first(where: { $0.field == rule.field })?.valueSource != nil {
                wanted[rule.field, default: []].formUnion(rule.propertyValues ?? [])
            }
            group.groups?.forEach(collect)
        }
        if let root = query.where { collect(root) }
        var names: [String: String] = [:]
        for (field, values) in wanted {
            let found = (try? await session.library.workFieldValues(field: field, search: nil, ids: Array(values))) ?? []
            for value in found { names[nameKey(field, value.value)] = value.name }
        }
        return names
    }

    /// [query]'s rules in a line: "Comedy, no Animation, 1990 to 1999, Unplayed".
    static func summary(of query: WorkQuery, fields: [WorkField], names: [String: String]) -> String {
        guard let root = query.where else { return "" }
        return phrases(of: root, fields: fields, names: names).joined(separator: root.match == "all" ? ", " : String(localized: " or "))
    }

    private static func phrases(of group: RuleGroup, fields: [WorkField], names: [String: String]) -> [String] {
        let rules = (group.rules ?? []).filter { $0.field != "library" }.compactMap { rule -> String? in
            fields.first { $0.field == rule.field }.map { phrase(rule, of: $0, names: names) }
        }
        let groups = (group.groups ?? []).map { group -> String in
            let inner = phrases(of: group, fields: fields, names: names)
                .joined(separator: group.match == "all" ? String(localized: " and ") : String(localized: " or "))
            return "(\(inner))"
        }
        return rules + groups
    }

    private static func phrase(_ rule: Rule, of field: WorkField, names: [String: String]) -> String {
        let draft = RuleDraft(field: rule.field, op: rule.op, arity: Int(field.ops.first { $0.op == rule.op }?.arity ?? -1), values: rule.propertyValues ?? [])
        let label = Self.field(field)
        switch rule.op {
        case "anyOf", "allOf": return values(draft, of: field, names: names)
        case "noneOf": return String(localized: "no \(values(draft, of: field, names: names))")
        case "isSet": return String(localized: "has \(label.lowercased())")
        case "isNotSet": return String(localized: "no \(label.lowercased())")
        case "between":
            let range = values(draft, of: field, names: names, to: String(localized: "to"))
            return field.field == "year" ? range : "\(label) \(range)"
        default:
            return "\(label) \(op(rule.op, type: field.type)) \(values(draft, of: field, names: names))"
        }
    }
}
