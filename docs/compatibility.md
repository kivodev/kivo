# Server and app compatibility

Kivo has one version. The server, the web app inside it and every app are released together under it, such as 0.6.0. An admin updates the server when they choose, and apps update from their stores, so a server and an app are often on different versions.

## The rules

- The server supports every app released in the last three months. It never removes or renames a call that a supported app uses.
- An app works with servers up to six months old. It hides a feature that an older server lacks.

## The check

Each side states the oldest version of the other that it works with.

- The server states the oldest app it supports as `minAppVersion` in `GET /api/v1/server`, which an app reads before sign-in.
- Each app holds the oldest server it works with.

An app compares the two as it connects to a server, before sign-in, and again each time it reconnects, as it does when the server restarts on another version.

- The server's version is older than the app's oldest server: the app shows **This server needs an update**. The person who runs the server updates it.
- The app's version is older than the server's `minAppVersion`: the app shows **Update Kivo**. The person updates the app.
- Otherwise the app shows nothing and goes on.

Either screen takes the place of sign-in and of that server's screens. **Try again** asks the server what it is again, and **Change server** goes to another server. The app stays signed in. Downloads stay on the phone, and the Downloads tab still shows and plays them.

The check compares versions by major, minor and patch alone. A leading `v` and anything from the first hyphen or plus on are ignored, so a dev build such as `0.6.1-dev+56c0018d` is 0.6.1 and an app's `0.6.0+78` is 0.6.0, and a prerelease is never older than the release it names. A version without the three numbers, such as `dev` or a commit hash, and a server that states no `minAppVersion`, never block: the app goes on.

## Raising a minimum

Both minimums start at the first public release and change only on purpose.

To stop supporting older apps, raise `MinApp` in `server/internal/version/minapp.go` to the oldest app release that still works, in the change that removes what the older apps used. It can't be newer than the latest release, which a test checks. Say so in the release notes: people on older apps see **Update Kivo** once their server updates.

To make the apps need a newer server, raise `MIN_SERVER_VERSION` in `clients/core/src/commonMain/kotlin/com/mmcdole/kivo/core/Compatibility.kt` for the iPhone, iPad, Android and Android TV apps, and `minServerVersion` in `server/web/src/tv/compatibility.ts` for the Samsung TV app, to the release whose server the app needs. Prefer hiding the feature on older servers, by comparing the server's version with `compareVersions`, and raise the minimum only when the app can't work at all without the newer server.

Keep to the rules above when raising either: a minimum that cuts off an app newer than three months, or a server newer than six, breaks them.

## The web app and the Cast receiver

The web app ships inside the server, so it is always the server's version and makes no check.

The Cast receiver is hosted and always the newest. It and the server state a Cast protocol number (`castProtocol` in `GET /api/v1/server`), and a receiver that speaks another says which side to update.
