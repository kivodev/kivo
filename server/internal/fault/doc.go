// Package fault gives errors three things. A kind says whose fault the
// error is and how a response reports it. A code tells it apart from other
// errors of its kind. A field names the part of the request it is about, if
// any. A sentinel error that a response reports by its kind or code is
// made with it. One that can only ever be a 500 is a plain error.
package fault
