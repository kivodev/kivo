package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.Credit
import com.mmcdole.kivo.api.models.PersonSummary

/**
 * The credit roles the apps have words for, by [value]. A role is the
 * server's free text, so any other is [Unknown] and shows as it came.
 */
enum class CreditRole(val value: String) {
    Actor("actor"),
    Director("director"),
    Writer("writer"),
    Author("author"),
    Narrator("narrator"),
    Unknown(""),
    ;

    override fun toString(): String = value

    companion object {
        /** The role [value] names; [Unknown] for any other. */
        fun of(value: String?): CreditRole = entries.firstOrNull { it != Unknown && it.value == value } ?: Unknown
    }
}

/** The credit's role, when it's one the apps have words for. */
val Credit.knownRole: CreditRole get() = CreditRole.of(role)

/** The role they have on the most works, when it's one the apps have words for. */
val PersonSummary.knownRole: CreditRole get() = CreditRole.of(role)
