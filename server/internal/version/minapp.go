package version

// MinApp is the oldest app release this server supports, which it states
// in its server info. An older app tells its user to update rather than
// call a server that no longer answers as it expects. Raise it in the
// change that ends support for older apps, to the oldest release that
// still works.
const MinApp = "0.3.0"
