import KivoCore
import KivoUI
import SwiftUI

/// The smart list builder, a sheet over the Lists tab, a smart list's page
/// or a filtered grid: the name, what to show, which libraries, the rules
/// and groups, the sort, and how many works match, driven by the server's
/// field catalog. A saved query this app can't edit whole is left alone,
/// and says so.
struct SmartListBuilder: View {
    private let list: WorkList?
    private let name: String
    private let query: WorkQuery?
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    /// The catalog's fields of the types this app can edit.
    @State private var fields: [WorkField]?
    @State private var draft: SmartListDraft?
    @State private var refusal: SmartListRefusal?
    @State private var failure: String?
    @State private var problem: String?

    private static let fieldTypes: Set<String> = ["choice", "set", "person", "number", "date", "text"]

    /// Opens the builder on [list]'s query.
    init(editing list: WorkList) {
        self.list = list
        name = list.name
        query = list.smart?.query
    }

    /// Opens the builder for a new list, starting from [query] and [name] when given.
    init(name: String = "", query: WorkQuery? = nil) {
        list = nil
        self.name = name
        self.query = query
    }

    var body: some View {
        NavigationStack {
            content
                .scrollContentBackground(.hidden)
                .navigationTitle(list == nil ? String(localized: "New Smart List") : String(localized: "Edit Smart List"))
                .toolbarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "Cancel")) { dismiss() }
                    }
                    if let draft {
                        ToolbarItem(placement: .confirmationAction) {
                            Button(String(localized: "Save"), action: save)
                                .disabled(!draft.isReady)
                                .accessibilityIdentifier("smart-list-save")
                        }
                    }
                }
                .kivoPageBackground()
        }
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
        .task { await load() }
    }

    @ViewBuilder private var content: some View {
        if let refusal {
            MessageView(
                String(localized: "This list can't be edited here."),
                sentence: refusal == .newerVersion
                    ? String(localized: "It was made by a newer version of Kivo. It stays as it is.")
                    : String(localized: "It uses a rule this version of Kivo doesn't know. It stays as it is.")
            )
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let fields, draft != nil {
            SmartListForm(draft: Binding($draft)!, fields: fields, startingQuery: query)
        } else if let failure {
            Form {
                Section { FailureRow(message: failure) { Task { await load() } } }
            }
        } else {
            LoadingView(String(localized: "Loading…"))
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func load() async {
        do {
            let catalog = try await server.session.library.workFields().filter { Self.fieldTypes.contains($0.type) }
            fields = catalog
            failure = nil
            if let query, let refusal = SmartLists.shared.refusal(query: query, fields: catalog) {
                self.refusal = refusal
            } else {
                draft = SmartLists.shared.draft(name: name, query: query, fields: catalog)
            }
        } catch {
            failure = error.problemSentence
        }
    }

    private func save() {
        guard let draft else { return }
        let name = draft.name.trimmingCharacters(in: .whitespacesAndNewlines)
        let query = draft.query
        Task {
            do {
                if let list {
                    _ = try await server.session.library.updateList(id: list.id, name: name, query: query)
                } else {
                    let made = try await server.session.library.createSmartList(name: name, query: query)
                    server.paths[server.tab, default: []].append(made.route)
                }
                dismiss()
            } catch {
                problem = error.listNameSentence
            }
        }
    }
}

/// The builder's form: Name, Show and Libraries, the rules, the groups,
/// Sort and the live count; each rule is a row opening its own page.
private struct SmartListForm: View {
    @Binding var draft: SmartListDraft
    let fields: [WorkField]
    let startingQuery: WorkQuery?
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    /// The rule whose page is open.
    @State private var editing: String?
    /// Chosen values' names, by `SmartLists.nameKey`.
    @State private var names: [String: String] = [:]
    /// The live count as a line, nil before the first answer or with no kinds.
    @State private var matching: String?

    var body: some View {
        Form {
            Section {
                TextField(String(localized: "Name"), text: Binding(get: { draft.name }, set: { draft = draft.named(name: $0) }))
                    .accessibilityIdentifier("smart-list-name")
            }
            Section {
                NavigationLink {
                    KindsPage(draft: $draft, offered: offeredKinds)
                } label: {
                    LabeledContent(String(localized: "Show"), value: draft.kinds.map(SmartListWords.kind).joined(separator: ", "))
                }
                .accessibilityIdentifier("smart-list-show")
                // A root matching any would OR the libraries with the rules.
                if draft.match == "all" {
                    NavigationLink {
                        LibrariesPage(draft: $draft)
                    } label: {
                        LabeledContent(String(localized: "Libraries"), value: librariesLabel)
                    }
                    .accessibilityIdentifier("smart-list-libraries")
                }
            }
            Section {
                ForEach(draft.rules, id: \.id) { rule in
                    row(rule)
                }
                .onDelete { remove(draft.rules, at: $0) }
                addRule(toGroup: nil)
                Button(String(localized: "Add Group")) {
                    guard let field = offeredFields(keeping: nil).first else { return }
                    draft = draft.addingGroup(group: SmartLists.shared.blankGroup(field: field))
                }
                .accessibilityIdentifier("add-group")
            } header: {
                Text(draft.match == "any" ? String(localized: "Match Any of These") : String(localized: "Match All of These"))
            }
            ForEach(draft.groups, id: \.id) { group in
                Section {
                    ForEach(group.rules, id: \.id) { rule in
                        row(rule)
                    }
                    .onDelete { remove(group.rules, at: $0) }
                    addRule(toGroup: group.id)
                    Button(String(localized: "Remove Group"), role: .destructive) {
                        draft = draft.removingGroup(id: group.id)
                    }
                } header: {
                    groupHeader(group)
                }
            }
            Section {
                sortPicker
            } footer: {
                if let matching {
                    Text(matching)
                        .monospacedDigit()
                        .accessibilityIdentifier("smart-list-count")
                }
            }
        }
        // Each choice pushes its own list with a checkmark, as in the Settings app.
        .pickerStyle(.navigationLink)
        .scrollDismissesKeyboard(.interactively)
        .navigationDestination(item: $editing) { id in
            RulePage(
                rule: Binding(get: { draft.rule(id: id)! }, set: { new in draft = draft.changingRule(id: id) { _ in new } }),
                fields: offeredFields(keeping: draft.rule(id: id)?.field),
                names: $names,
                session: server.session
            )
        }
        .task {
            if let startingQuery {
                names = (try? await server.session.library.smartListNames(query: startingQuery, fields: fields)) ?? [:]
            }
        }
        .task(id: draft.query.description) { await count() }
    }

    private var offeredKinds: [String] {
        var seen: Set<String> = []
        return fields.flatMap(\.kinds).filter { seen.insert($0).inserted }
    }

    /// The fields a rule can use for the kinds showing, and the one a rule already has.
    private func offeredFields(keeping kept: String?) -> [WorkField] {
        fields.filter { field in
            field.field == kept || (field.field != "library" && !Set(field.kinds).isDisjoint(with: draft.kinds))
        }
    }

    /// Removes the rules at [offsets] of [rules], the root's or a group's.
    private func remove(_ rules: [RuleDraft], at offsets: IndexSet) {
        offsets.map { rules[$0].id }.forEach { draft = draft.removingRule(id: $0) }
    }

    private var librariesLabel: String {
        if draft.libraries.isEmpty { return String(localized: "All") }
        return server.libraries.filter { draft.libraries.contains($0.id) }.map(\.name).joined(separator: ", ")
    }

    /// A rule's row, opening its page: "Genre is Comedy".
    private func row(_ rule: RuleDraft) -> some View {
        Button {
            editing = rule.id
        } label: {
            HStack(spacing: KivoTokens.spacing * 1.5) {
                if let field = fields.first(where: { $0.field == rule.field }) {
                    Text(SmartListWords.field(field))
                        .foregroundStyle(palette.text)
                    Text(SmartListWords.op(rule.op, type: field.type))
                        .foregroundStyle(palette.textMuted)
                    Text(rule.isComplete ? SmartListWords.values(rule, of: field, names: names) : String(localized: "Choose"))
                        .foregroundStyle(rule.isComplete ? palette.text : palette.textMuted)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(palette.textMuted)
            }
            .lineLimit(1)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("rule:\(rule.field)")
    }

    /// A rule on the first field the kinds offer, its values not chosen yet.
    private var blankRule: RuleDraft? {
        offeredFields(keeping: nil).first.flatMap { SmartLists.shared.blankRule(field: $0) }
    }

    /// Add Rule: a blank rule in [groupId] or the root's, opened at once.
    private func addRule(toGroup groupId: String?) -> some View {
        Button(String(localized: "Add Rule")) {
            guard let rule = blankRule else { return }
            draft = draft.addingRule(rule: rule, groupId: groupId)
            editing = rule.id
        }
        .accessibilityIdentifier("add-rule")
    }

    /// A group's heading, a menu of what the group matches.
    private func groupHeader(_ group: GroupDraft) -> some View {
        Menu {
            Picker(String(localized: "Match"), selection: Binding(get: { group.match }, set: { draft = draft.withGroupMatch(id: group.id, match: $0) })) {
                Text("Match Any of These").tag("any")
                Text("Match All of These").tag("all")
            }
            .pickerStyle(.inline)
        } label: {
            HStack(spacing: 3) {
                Text(group.match == "all" ? String(localized: "Match All of These") : String(localized: "Match Any of These"))
                Image(systemName: "chevron.up.chevron.down").font(.caption2.weight(.semibold))
            }
        }
        .textCase(nil)
        .accessibilityIdentifier("group-match")
    }

    /// The sorts: each sortable field the kinds offer, and the one saved,
    /// in either direction, its own first, and Random.
    private var sortPicker: some View {
        let current = draft.sort.first
        let currentTag = current.map { key in
            key.field == SmartLists.shared.RANDOM_SORT ? key.field : "\(key.field):\(direction(of: key))"
        } ?? "title:asc"
        let chosenField = current?.field ?? "title"
        var choices: [(tag: String, label: String, keys: [SortKey])] = fields
            .filter { $0.sortable && ($0.field == chosenField || !Set($0.kinds).isDisjoint(with: draft.kinds)) }
            .flatMap { field -> [(tag: String, label: String, keys: [SortKey])] in
                let own = field.defaultDirection ?? "asc"
                return [own, own == "asc" ? "desc" : "asc"].map { way in
                    ("\(field.field):\(way)", SmartListWords.sort(field, direction: way), [SortKey(field: field.field, direction: way)])
                }
            }
        choices.append((SmartLists.shared.RANDOM_SORT, String(localized: "Random"), [SortKey(field: SmartLists.shared.RANDOM_SORT, direction: nil)]))
        return Picker(String(localized: "Sort"), selection: Binding(
            get: { currentTag },
            set: { tag in draft = draft.sorted(sort: choices.first { $0.tag == tag }?.keys ?? []) }
        )) {
            ForEach(choices, id: \.tag) { Text($0.label).tag($0.tag) }
        }
        .accessibilityIdentifier("smart-list-sort")
    }

    private func direction(of key: SortKey) -> String {
        key.direction ?? fields.first { $0.field == key.field }?.defaultDirection ?? "asc"
    }

    /// Reads the count a moment after the rules stop changing.
    private func count() async {
        guard !draft.kinds.isEmpty else {
            matching = nil
            return
        }
        do {
            try await Task.sleep(for: .milliseconds(350))
            let query = draft.query
            let total = Int(try await server.session.library.countMatching(query: query))
            let line = SmartListWords.count(total, kinds: query.kinds)
            matching = total == 1 ? String(localized: "\(line) matches") : String(localized: "\(line) match")
        } catch {
            guard !Task.isCancelled else { return }
            matching = error.problemSentence
        }
    }
}

/// Show: the kinds the catalog offers, in its order, any number of them.
private struct KindsPage: View {
    @Binding var draft: SmartListDraft
    let offered: [String]

    var body: some View {
        List {
            ForEach(offered, id: \.self) { kind in
                ChoiceRow(title: SmartListWords.kind(kind), detail: nil, chosen: draft.kinds.contains(kind)) {
                    let others = draft.kinds.contains(kind) ? draft.kinds.filter { $0 != kind } : draft.kinds + [kind]
                    draft = draft.showing(kinds: offered.filter(others.contains))
                }
                .accessibilityIdentifier("kind:\(kind)")
            }
        }
        .scrollContentBackground(.hidden)
        .navigationTitle(String(localized: "Show"))
        .toolbarTitleDisplayMode(.inline)
        .kivoPageBackground()
    }
}

/// Libraries: All, or any of the libraries the user sees.
private struct LibrariesPage: View {
    @Binding var draft: SmartListDraft
    @Environment(ServerModel.self) private var server

    var body: some View {
        List {
            ChoiceRow(title: String(localized: "All"), detail: nil, chosen: draft.libraries.isEmpty) {
                draft = draft.inLibraries(libraries: [])
            }
            ForEach(server.libraries, id: \.id) { library in
                ChoiceRow(title: library.name, detail: nil, chosen: draft.libraries.contains(library.id)) {
                    draft = draft.inLibraries(libraries: draft.libraries.symmetricDifference([library.id]))
                }
            }
        }
        .scrollContentBackground(.hidden)
        .navigationTitle(String(localized: "Libraries"))
        .toolbarTitleDisplayMode(.inline)
        .kivoPageBackground()
    }
}

/// A rule's page: its field, its operator, then values by the operator's
/// arity, chosen from the field's values or typed.
private struct RulePage: View {
    @Binding var rule: RuleDraft
    let fields: [WorkField]
    @Binding var names: [String: String]
    let session: KivoSession
    /// The unit of a period, one of [units].
    @State private var unit = "D"
    @State private var search = ""
    /// The field's values, as the search found them.
    @State private var found: [WorkFieldValue] = []

    private static let units: [(code: String, label: String)] = [
        ("D", String(localized: "Days")),
        ("W", String(localized: "Weeks")),
        ("M", String(localized: "Months")),
        ("Y", String(localized: "Years")),
    ]

    private var field: WorkField? { fields.first { $0.field == rule.field } }

    var body: some View {
        Form {
            Section {
                Picker(String(localized: "Field"), selection: Binding(get: { rule.field }, set: choose)) {
                    ForEach(fields, id: \.field) { Text(SmartListWords.field($0)).tag($0.field) }
                }
                .accessibilityIdentifier("rule-field")
                if let field {
                    Picker(String(localized: "Condition"), selection: Binding(get: { rule.op }, set: chooseOp)) {
                        ForEach(field.ops, id: \.op) { Text(SmartListWords.op($0.op, type: field.type)).tag($0.op) }
                    }
                    .accessibilityIdentifier("rule-op")
                }
            }
            if let field, rule.arity != 0 {
                values(field)
            }
        }
        .pickerStyle(.navigationLink)
        .scrollContentBackground(.hidden)
        .scrollDismissesKeyboard(.interactively)
        .navigationTitle(field.map(SmartListWords.field) ?? "")
        .toolbarTitleDisplayMode(.inline)
        .kivoPageBackground()
        // On the form: a section with no rows yet never appears, so it couldn't start its own read.
        .task(id: "\(rule.field)/\(search)") { await readValues() }
    }

    /// Reads the field's values, a moment after a search stops changing.
    private func readValues() async {
        guard let field, field.valueSource != nil else { return }
        if !search.isEmpty {
            do {
                try await Task.sleep(for: .milliseconds(350))
            } catch {
                return
            }
        }
        let trimmed = search.trimmingCharacters(in: .whitespaces)
        if let values = try? await session.library.workFieldValues(field: field.field, search: trimmed.isEmpty ? nil : trimmed, ids: nil) {
            found = values
        }
    }

    /// A new field keeps the rule's operator if it has it, its first if not; the values start over.
    private func choose(_ newField: String) {
        guard newField != rule.field, let field = fields.first(where: { $0.field == newField }) else { return }
        search = ""
        found = []
        rule = rule.withField(field: field)
    }

    /// A new operator keeps values it reads the same way.
    private func chooseOp(_ newOp: String) {
        guard let field, let op = field.ops.first(where: { $0.op == newOp }) else { return }
        rule = rule.withOp(op: op)
    }

    @ViewBuilder private func values(_ field: WorkField) -> some View {
        if SmartLists.shared.isPeriod(op: rule.op) {
            periodSection
        } else if field.valueSource != nil {
            ValueChooser(rule: $rule, field: field, names: $names, search: $search, found: found)
        } else {
            Section {
                ForEach(rule.values.indices, id: \.self) { index in
                    TextField(placeholder(field, index), text: text(field, at: index))
                        .keyboardType(field.type == "number" ? .decimalPad : .default)
                        .autocorrectionDisabled()
                        .accessibilityIdentifier("rule-value-\(index)")
                }
            } footer: {
                if field.type == "date" {
                    Text("Dates are YYYY-MM-DD.")
                } else if field.unit == "ms" {
                    Text("In minutes.")
                }
            }
        }
    }

    private func placeholder(_ field: WorkField, _ index: Int) -> String {
        if rule.arity == 2 { return index == 0 ? String(localized: "From") : String(localized: "To") }
        return field.type == "date" ? "YYYY-MM-DD" : String(localized: "Value")
    }

    /// The value at [index] as typed; a duration is typed in minutes and held in milliseconds.
    private func text(_ field: WorkField, at index: Int) -> Binding<String> {
        Binding(
            get: { field.unit == "ms" && !rule.values[index].isEmpty ? SmartListWords.minutes(rule.values[index]) : rule.values[index] },
            set: { typed in
                let value = typed.trimmingCharacters(in: .whitespaces)
                var values = rule.values
                values[index] = field.unit == "ms" ? Double(value).map { String(Int($0 * 60000)) } ?? "" : value
                rule = rule.withValues(values: values)
            }
        )
    }

    /// A period: a number of days, weeks, months or years, held as P30D.
    private var periodSection: some View {
        let amount = Binding(
            get: { SmartLists.shared.periodAmount(value: rule.values.first ?? "") },
            set: { (typed: String) in
                rule = rule.withValues(values: [SmartLists.shared.period(amount: typed.filter(\.isNumber), unit: unit)])
            }
        )
        return Section {
            TextField(String(localized: "Number"), text: amount)
                .keyboardType(.numberPad)
                .accessibilityIdentifier("rule-value-0")
            Picker(String(localized: "Unit"), selection: Binding(get: { unit }, set: { newUnit in
                unit = newUnit
                if let value = rule.values.first, !value.isEmpty {
                    rule = rule.withValues(values: [SmartLists.shared.period(amount: SmartLists.shared.periodAmount(value: value), unit: newUnit)])
                }
            })) {
                ForEach(Self.units, id: \.code) { Text($0.label).tag($0.code) }
            }
        }
        .onAppear {
            if rule.values.first?.isEmpty == false { unit = SmartLists.shared.periodUnit(value: rule.values.first) }
        }
    }
}

/// A rule's values chosen from the field's: its inline choices, or for a
/// field with many, the ones chosen and what a search finds.
private struct ValueChooser: View {
    @Binding var rule: RuleDraft
    let field: WorkField
    @Binding var names: [String: String]
    @Binding var search: String
    let found: [WorkFieldValue]

    private var searches: Bool { field.valueSource == "search" }

    var body: some View {
        if searches {
            Section {
                TextField(String(localized: "Search"), text: $search)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("value-search")
            }
            if !rule.values.isEmpty {
                Section(String(localized: "Chosen")) {
                    ForEach(rule.values, id: \.self) { value in
                        ChoiceRow(title: names[SmartLists.shared.nameKey(field: field.field, value: value)] ?? value, detail: nil, chosen: true) {
                            rule = rule.withValues(values: rule.values.filter { $0 != value })
                        }
                    }
                }
            }
        }
        Section {
            ForEach(found.filter { !searches || !rule.values.contains($0.value) }, id: \.value) { value in
                ChoiceRow(title: value.name, detail: nil, chosen: rule.values.contains(value.value)) { toggle(value) }
                    .accessibilityIdentifier("value:\(value.name)")
            }
        }
    }

    private func toggle(_ value: WorkFieldValue) {
        names[SmartLists.shared.nameKey(field: field.field, value: value.value)] = value.name
        rule = rule.withValues(values: rule.values.contains(value.value) ? rule.values.filter { $0 != value.value } : rule.values + [value.value])
    }
}
