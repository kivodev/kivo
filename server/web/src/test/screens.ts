// The screens that the routes load lazily. They are loaded along with any
// test file that renders the app, so a test's time is never spent
// compiling a screen while other workers compile theirs. knip leaves this
// file out, because its glob would count every export of the features as
// used.
await Promise.all(
  Object.values(import.meta.glob(["/src/features/**/*.{ts,tsx}", "!**/*.test.{ts,tsx}"])).map((load) => load()),
);
