import UIKit

/// The taps iOS gives for an action that worked.
enum Haptics {
    static func success() {
        UINotificationFeedbackGenerator().notificationOccurred(.success)
    }
}
