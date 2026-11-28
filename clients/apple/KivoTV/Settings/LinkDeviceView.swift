import KivoCore
import KivoUI
import SwiftUI

/// Link a device: the code another device shows, typed with the TV's
/// keyboard, then "Allow Living Room to sign in as anna?" with what the
/// server saw of it, a warning when it isn't on the home network, and
/// Don't allow, focused first, beside Allow; then what was done
/// (LinkDeviceModel).
struct LinkDeviceView: View {
    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    @State private var model: LinkDeviceModel?
    @State private var typed = ""
    @FocusState private var dontAllow: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 8) {
            if let model {
                content(model)
                if let message = model.message {
                    Text(message)
                        .kivoStyle(KivoTokens.TextSize.base)
                        .foregroundStyle(palette.danger)
                }
            }
        }
        .frame(width: 1000, alignment: .leading)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .navigationTitle(String(localized: "Link a device"))
        .task {
            if model == nil { model = LinkDeviceModel(core: app.core, session: server.session) }
        }
    }

    @ViewBuilder private func content(_ model: LinkDeviceModel) -> some View {
        switch model.step {
        case .enter:
            Text("Enter the code the other device shows.")
                .kivoStyle(KivoTokens.TextSize.base)
                .foregroundStyle(palette.textMuted)
            TextField(String(localized: "Code"), text: $typed)
                .textInputAutocapitalization(.characters)
                .autocorrectionDisabled()
                .onSubmit { Task { await model.look(up: typed) } }
            Button(String(localized: "Continue")) { Task { await model.look(up: typed) } }
                .disabled(typed.isEmpty || model.busy)
        case let .otherServer(name):
            Text(name.map { String(localized: "This device is signing in to \($0).") } ?? String(localized: "This device is signing in to another server."))
                .kivoStyle(KivoTokens.TextSize.xl, weight: KivoTokens.Weight.semibold)
            Text("You're signed in to \(server.serverName). Sign in to that server here to link it.")
                .kivoStyle(KivoTokens.TextSize.base)
                .foregroundStyle(palette.textMuted)
            Button(String(localized: "Done")) { dismiss() }
        case let .confirm(code, request):
            Text("Allow \(request.name) to sign in as \(server.user?.username ?? "")?")
                .kivoStyle(KivoTokens.TextSize.xl, weight: KivoTokens.Weight.semibold)
            Text(LinkDeviceModel.context(of: request))
                .kivoStyle(KivoTokens.TextSize.base)
                .foregroundStyle(palette.textMuted)
            if !request.nearby {
                Label(
                    String(localized: "This device isn't on your home network. Allow it only if it's yours and you just started signing in on it."),
                    systemImage: "exclamationmark.triangle.fill"
                )
                .font(.kivo(KivoTokens.TextSize.base))
                .foregroundStyle(palette.danger)
            }
            HStack(spacing: KivoTokens.spacing * 6) {
                Button(String(localized: "Don't allow")) { Task { await model.answer(code: code, request: request, allow: false) } }
                    .focused($dontAllow)
                Button(String(localized: "Allow")) { Task { await model.answer(code: code, request: request, allow: true) } }
            }
            .disabled(model.busy)
            .onAppear { dontAllow = true }
        case let .done(name, allowed):
            Text(allowed ? String(localized: "\(name) is signed in") : String(localized: "\(name) wasn't signed in"))
                .kivoStyle(KivoTokens.TextSize.xl, weight: KivoTokens.Weight.semibold)
            Button(String(localized: "Done")) { dismiss() }
        }
    }
}
