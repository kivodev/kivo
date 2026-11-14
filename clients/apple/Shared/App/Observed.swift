import KivoCore
import Observation

/// A core flow's latest value, for a view or a model to read: it follows
/// the flow on the main actor from the value it has now, and stops as it's
/// released. [onChange] is told each value as it's taken, the first too.
@MainActor
@Observable
final class Observed<Value> {
    private(set) var value: Value
    @ObservationIgnored private var following: Task<Void, Never>?

    /// Follows a core `StateFlow` from its value now.
    convenience init(_ flow: SkieSwiftStateFlow<Value>, onChange: ((Value) -> Void)? = nil) {
        self.init(flow.value, flow, onChange: onChange)
    }

    /// Follows [values], a core `Flow`, from [initial], which shows until its first value.
    init<Values: AsyncSequence>(_ initial: Value, _ values: Values, onChange: ((Value) -> Void)? = nil) where Values.Element == Value {
        value = initial
        following = Task { [weak self] in
            do {
                for try await next in values {
                    guard let self else { return }
                    self.value = next
                    onChange?(next)
                }
            } catch {
                // A core flow doesn't fail; one that does stays at its last value.
            }
        }
    }

    deinit {
        following?.cancel()
    }
}
