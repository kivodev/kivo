// Package activity keeps a record, for admins, of what happened on the
// server: plays started and finished, root works added, sign-ins and failed
// sign-ins, users joining, task runs ending and problems raised. The
// component that makes a change records its event in the change's own
// transaction, so an event exists exactly when its change does. Events are
// kept for 90 days.
package activity
