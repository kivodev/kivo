// Package fault gives errors three things. A kind says whose fault the
// error is and how a response reports it. A code tells it apart from other
// errors of its kind. A field names the part of the request it is about, if
// any. Every package makes its sentinel errors with it.
package fault
