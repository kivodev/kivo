import KivoCore
import KivoUI
import SwiftUI

/// A page of settings: [account]'s sections of the account's playback
/// settings, kept on the server, once they're read; then [device]'s
/// sections of this device's own; and the last change's failure, if any.
struct SettingsForm<Account: View, Device: View>: View {
    let title: String
    /// Whether the page reads the account's settings.
    private let readsAccount: Bool
    private let account: (PlaybackPreferences, AccountSave) -> Account
    private let device: (DeviceSettings, DeviceSave) -> Device
    @Environment(ServerModel.self) private var server
    @Environment(AppModel.self) private var app
    /// The user as the server last said.
    @State private var user: CurrentUser?
    /// Why reading the user failed.
    @State private var loadFailure: String?
    @State private var accountFailure: SaveFailure?
    @State private var deviceFailure: SaveFailure?

    init(
        title: String,
        @ViewBuilder account: @escaping (PlaybackPreferences, AccountSave) -> Account,
        @ViewBuilder device: @escaping (DeviceSettings, DeviceSave) -> Device
    ) {
        self.init(title: title, readsAccount: true, account: account, device: device)
    }

    private init(
        title: String,
        readsAccount: Bool,
        account: @escaping (PlaybackPreferences, AccountSave) -> Account,
        device: @escaping (DeviceSettings, DeviceSave) -> Device
    ) {
        self.title = title
        self.readsAccount = readsAccount
        self.account = account
        self.device = device
    }

    var body: some View {
        Form {
            if readsAccount {
                if let playback = user?.playback {
                    account(playback, AccountSave(run: save))
                } else if let loadFailure {
                    Section {
                        FailureRow(message: loadFailure) { Task { await load() } }
                    } header: {
                        AccountHeading()
                    }
                } else {
                    Section {
                        ProgressView().frame(maxWidth: .infinity)
                    } header: {
                        AccountHeading()
                    }
                }
            }
            if let settings = app.settings {
                device(settings, DeviceSave(run: keep))
            }
            if let accountFailure {
                Section {
                    FailureRow(message: String(localized: "Couldn't save. \(accountFailure.message)"), retry: accountFailure.retry)
                }
            }
            if let deviceFailure {
                Section {
                    FailureRow(message: String(localized: "Couldn't save. \(deviceFailure.message)"), retry: deviceFailure.retry)
                }
            }
        }
        // Each choice pushes its own list with a checkmark, as in the Settings app.
        .pickerStyle(.navigationLink)
        .navigationTitle(title)
        .toolbarTitleDisplayMode(.inline)
        .task {
            if readsAccount { await load() }
        }
    }

    private func load() async {
        do {
            user = try await server.session.account.currentUser()
            loadFailure = nil
        } catch {
            loadFailure = error.problemSentence
        }
    }

    /// Sends a change to the server and keeps the user it answers with.
    private func save(_ change: @escaping () async throws -> CurrentUser) {
        Task {
            do {
                user = try await change()
                accountFailure = nil
            } catch {
                accountFailure = SaveFailure(message: error.problemSentence) { save(change) }
            }
        }
    }

    private func keep(_ change: @escaping () async throws -> Void) {
        Task {
            do {
                try await change()
                deviceFailure = nil
            } catch {
                deviceFailure = SaveFailure(message: error.problemSentence) { keep(change) }
            }
        }
    }
}

extension SettingsForm where Device == EmptyView {
    /// A page of the account's settings only.
    init(title: String, @ViewBuilder account: @escaping (PlaybackPreferences, AccountSave) -> Account) {
        self.init(title: title, readsAccount: true, account: account) { _, _ in EmptyView() }
    }
}

extension SettingsForm where Account == EmptyView {
    /// A page of this device's settings only.
    init(title: String, @ViewBuilder device: @escaping (DeviceSettings, DeviceSave) -> Device) {
        self.init(title: title, readsAccount: false, account: { _, _ in EmptyView() }, device: device)
    }
}

/// The heading over a page's settings of the account, which follow the user to every device.
struct AccountHeading: View {
    var body: some View {
        Text("All your devices")
    }
}

/// The heading over a page's settings of this device: "This iPhone", or "This iPad" on an iPad.
struct DeviceHeading: View {
    var body: some View {
        Text(ThisDevice.name)
    }
}

/// Sends a change to the account's settings to the server; a refusal leaves
/// the saved value showing and says why under the page's settings, with Retry.
struct AccountSave {
    fileprivate let run: (@escaping () async throws -> CurrentUser) -> Void

    /// A binding showing [value] that sends each change with [send].
    func binding<Value>(_ value: Value, send: @escaping (Value) async throws -> CurrentUser) -> Binding<Value> {
        Binding(get: { value }, set: { newValue in run { try await send(newValue) } })
    }
}

/// Keeps a change to this device's settings, and on failure says why under the page's settings, with Retry.
struct DeviceSave {
    fileprivate let run: (@escaping () async throws -> Void) -> Void

    /// A binding showing [value] that keeps each change with [send].
    func binding<Value>(_ value: Value, send: @escaping (Value) async throws -> Void) -> Binding<Value> {
        Binding(get: { value }, set: { newValue in run { try await send(newValue) } })
    }
}
