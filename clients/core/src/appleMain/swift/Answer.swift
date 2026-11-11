extension SkieSwiftFlow {
    /// A read's answer, once: the first it gives, or nil when that's a
    /// failure, a server that isn't answering among them, so it never waits
    /// for one to come back; nil too when the task is cancelled first.
    public func answer<Answer: AnyObject>() async -> Answer? where T == Load<Answer> {
        for await load in self {
            switch onEnum(of: load) {
            case .loading: continue
            case let .loaded(loaded): return loaded.value
            case .failed: return nil
            }
        }
        return nil
    }
}
