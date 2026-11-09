import Foundation
import KivoCore

/// A rule as the builder holds it: a field, an operator and values by the
/// operator's arity, with an empty slot for each exact value not typed yet.
struct RuleDraft: Identifiable {
    let id = UUID()
    var field: String
    var op: String
    var arity: Int
    var values: [String]

    /// Whether every value it takes is there.
    var isComplete: Bool {
        arity < 0 ? !values.isEmpty : values.count == arity && values.allSatisfy { !$0.isEmpty }
    }

    fileprivate var rule: Rule {
        Rule(field: field, op: op, propertyValues: values.isEmpty ? nil : values)
    }
}

/// A group of rules the root holds; Add Group starts one matching any.
struct GroupDraft: Identifiable {
    let id = UUID()
    var match = "any"
    var rules: [RuleDraft] = []
}

/// What the smart list builder edits: the name, the kinds, the libraries,
/// the rules and groups, and the sort. The max works and collapse a query
/// carries are kept untouched, and so is its sort until another is chosen.
struct SmartListDraft {
    var name: String
    var kinds: [String]
    /// The libraries of the `library` rule, empty for every library.
    var libraries: Set<String> = []
    private(set) var match = "all"
    var rules: [RuleDraft] = []
    var groups: [GroupDraft] = []
    var sort: [SortKey] = []
    private let maxWorks: KotlinInt?
    private let collapse: String?

    /// The query format this app knows.
    private static let queryVersion: Int32 = 1
    /// What a new list shows.
    private static let defaultKinds = ["movie"]

    /// A draft of [query] named [name], or of a new list when nil.
    init(name: String, query: WorkQuery?, fields: [WorkField]) {
        self.name = name
        kinds = query?.kinds ?? Self.defaultKinds
        maxWorks = query?.maxWorks
        collapse = query?.collapse
        sort = query?.sort ?? []
        guard let root = query?.where else { return }
        match = root.match
        var saved = root.rules ?? []
        if match == "all", let index = saved.firstIndex(where: { $0.field == "library" && $0.op == "anyOf" }) {
            libraries = Set(saved.remove(at: index).propertyValues ?? [])
        }
        rules = saved.map { Self.draft($0, fields) }
        groups = (root.groups ?? []).map { group in
            var draft = GroupDraft(match: group.match)
            draft.rules = (group.rules ?? []).map { Self.draft($0, fields) }
            return draft
        }
    }

    private static func draft(_ rule: Rule, _ fields: [WorkField]) -> RuleDraft {
        let op = fields.first { $0.field == rule.field }?.ops.first { $0.op == rule.op }
        return RuleDraft(field: rule.field, op: rule.op, arity: Int(op?.arity ?? -1), values: rule.propertyValues ?? [])
    }

    /// Whether a builder must leave [query] alone: a newer version, or a field missing from [fields].
    static func refuses(_ query: WorkQuery, fields: [WorkField]) -> Bool {
        if let version = query.version?.int32Value, version > queryVersion { return true }
        let known = Set(fields.map(\.field))
        let inRules = query.where.map(ruleFields) ?? []
        let inSort = (query.sort ?? []).map(\.field).filter { $0 != SmartListWords.randomSort }
        return !(inRules + inSort).allSatisfy(known.contains)
    }

    private static func ruleFields(_ group: RuleGroup) -> [String] {
        (group.rules ?? []).map(\.field) + (group.groups ?? []).flatMap(ruleFields)
    }

    var isReady: Bool {
        !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !kinds.isEmpty
            && (rules + groups.flatMap(\.rules)).allSatisfy(\.isComplete)
    }

    /// The query of the complete rules, for the count and for saving.
    var query: WorkQuery {
        let chosen = libraries.isEmpty ? [] : [Rule(field: "library", op: "anyOf", propertyValues: libraries.sorted())]
        let rootRules = chosen + rules.filter(\.isComplete).map(\.rule)
        let rootGroups = groups.compactMap { group -> RuleGroup? in
            let rules = group.rules.filter(\.isComplete).map(\.rule)
            return rules.isEmpty ? nil : RuleGroup(match: group.match, rules: rules, groups: nil)
        }
        let root = rootRules.isEmpty && rootGroups.isEmpty ? nil : RuleGroup(
            match: match,
            rules: rootRules.isEmpty ? nil : rootRules,
            groups: rootGroups.isEmpty ? nil : rootGroups
        )
        return WorkQuery(kinds: kinds, version: nil, where: root, sort: sort.isEmpty ? nil : sort, maxWorks: maxWorks, collapse: collapse)
    }

    subscript(rule id: UUID) -> RuleDraft {
        get {
            rules.first { $0.id == id } ?? groups.lazy.flatMap(\.rules).first { $0.id == id }!
        }
        set {
            if let index = rules.firstIndex(where: { $0.id == id }) {
                rules[index] = newValue
                return
            }
            for group in groups.indices {
                if let index = groups[group].rules.firstIndex(where: { $0.id == id }) {
                    groups[group].rules[index] = newValue
                }
            }
        }
    }
}

extension GridModel {
    /// The query for the grid showing: the kinds it lists, its library, its
    /// filters and sort, for Save as Smart List.
    var smartListQuery: WorkQuery {
        var rules = [Rule(field: "library", op: "anyOf", propertyValues: [library.id])]
        if let played = shown.played.value {
            rules.append(Rule(field: "playState", op: "anyOf", propertyValues: [played]))
        }
        if !shown.resolutions.isEmpty {
            rules.append(Rule(field: "resolution", op: "anyOf", propertyValues: shown.resolutions.sorted()))
        }
        if !shown.dynamicRanges.isEmpty {
            rules.append(Rule(field: "dynamicRange", op: "anyOf", propertyValues: shown.dynamicRanges.sorted()))
        }
        let kind = switch library.kind {
        case "movies": "movie"
        case "shows": "series"
        case "audiobooks": "book"
        default: "video"
        }
        return WorkQuery(
            kinds: [kind],
            version: nil,
            where: RuleGroup(match: "all", rules: rules, groups: nil),
            sort: Self.smartListSort(shown.sort.value),
            maxWorks: nil,
            collapse: nil
        )
    }

    /// The sort key of a grid's [sort], nil for one a query has no field for.
    private static func smartListSort(_ sort: String) -> [SortKey]? {
        switch sort {
        case "author": nil
        case "runtime": [SortKey(field: "runtimeMs", direction: nil)]
        default: [SortKey(field: sort, direction: nil)]
        }
    }
}
