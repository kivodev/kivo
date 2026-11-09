import Foundation
import KivoCore
import KivoUI

extension Error {
    /// The Kotlin exception behind an error thrown through the core, as
    /// Kotlin/Native puts it in the NSError's user info.
    var kotlinException: Any? {
        (self as NSError).userInfo["KotlinException"]
    }

    /// Why the call failed, as the core tells it: no answer from the
    /// server, signed out, or the server's problem. An error that isn't
    /// the core's is no answer.
    var requestProblem: RequestProblem {
        guard let throwable = kotlinException as? KotlinThrowable else { return RequestProblem.Unreachable.shared }
        return RequestProblem.companion.of(error: throwable)
    }

    /// The problem's code: the server's, or `network` when no answer came from it.
    var problemCode: String {
        requestProblem.code
    }

    /// The HTTP status of a refusal; nil when no answer came.
    var problemStatus: Int? {
        (kotlinException as? ProblemException).map { Int($0.status) }
    }

    /// The failure in a sentence the user can act on.
    var problemSentence: String {
        requestProblem.sentence
    }

    /// Why a list's name wasn't saved, in a sentence.
    var listNameSentence: String {
        problemCode == "nameTaken" ? String(localized: "You already have a list with that name.") : problemSentence
    }

    /// A failed subtitle search or download in a sentence; a quota used up
    /// says when there's more, in local time, when the server knows.
    var subtitleSearchSentence: String {
        guard let throwable = kotlinException as? KotlinThrowable else { return problemSentence }
        return Display.app.problem(problem: SubtitleSearch.shared.problem(error: throwable)) { ms in
            Date(timeIntervalSince1970: Double(ms.int64Value) / 1000).formatted(date: .omitted, time: .shortened)
        }
    }

    /// A download that couldn't start, in a sentence: too little room, the
    /// server refusing it, or the request's problem.
    var downloadSentence: String {
        guard let throwable = kotlinException as? KotlinThrowable else { return problemSentence }
        return Display.app.downloadProblem(error: throwable)
    }
}

extension RequestProblem {
    /// No answer came from the server.
    var isUnreachable: Bool {
        self is RequestProblem.Unreachable
    }

    /// The problem in a sentence the user can act on.
    var sentence: String {
        Display.app.problem(problem: self)
    }
}
