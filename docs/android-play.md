# Publishing the Android app on Google Play

The Android app is one app bundle for phones, tablets and TVs (`com.mmcdole.kivo`). This is the checklist for putting it on Google Play, starting with the internal testing track.

## The upload key and the release build

Google Play signs the app people install with its own app signing key (Play App Signing). You sign each bundle you upload with an upload key, which only proves the upload came from you. Keep the upload key and its passwords out of the repository.

1. Make the upload key, once, somewhere outside the repository:

   ```sh
   keytool -genkeypair -v -keystore ~/keys/kivo-upload.jks -alias upload \
     -keyalg RSA -keysize 4096 -validity 10000
   ```

   Back up the keystore and its passwords. If the upload key is lost, Google can reset it, but that takes a support request.

2. Tell Gradle where it is, in `~/.gradle/gradle.properties` (your own, not `clients/gradle.properties`):

   ```properties
   kivo.upload.storeFile=/home/you/keys/kivo-upload.jks
   kivo.upload.storePassword=...
   kivo.upload.keyAlias=upload
   kivo.upload.keyPassword=...
   ```

   Or set `KIVO_UPLOAD_STORE_FILE`, `KIVO_UPLOAD_STORE_PASSWORD`, `KIVO_UPLOAD_KEY_ALIAS` and `KIVO_UPLOAD_KEY_PASSWORD` in the environment. Without them, the release build is signed with the debug key, as sideloaded builds are.

3. Build the bundle from `clients/`, with a build number that's higher than any uploaded before for this version (the version code is version × 1000 + build number):

   ```sh
   ./gradlew :android:app:bundleRelease -PbuildNumber=1
   ```

   The bundle is `android/app/build/outputs/bundle/release/app-release.aab`. Check it's signed with the upload key:

   ```sh
   keytool -printcert -jarfile android/app/build/outputs/bundle/release/app-release.aab
   ```

The app Play installs is signed with Play's key, so it can't install over a sideloaded build signed with the debug key, or the other way round. Moving a device from one to the other means uninstalling first, which signs it out and deletes its downloads.

## The Play Console

1. Create a developer account at [play.google.com/console](https://play.google.com/console). It costs a one-off $25 and needs identity verification; an organization account also needs a D-U-N-S number. A new personal account has to run a closed test with at least 12 testers for 14 days before it can publish to production. Internal testing has no such wait.
2. Create the app: name Kivo, default language, App, Free.
3. Under **Test and release › Setup › App signing**, keep Play App Signing on (the default for new apps). The first bundle uploaded registers its key as the upload key.
4. Under **Testing › Internal testing**, create a testers list (up to 100 Google accounts by email), create a release, upload `app-release.aab`, and roll it out. Testers join from the opt-in link on that page, then install from the Play Store.
5. Fill in the sections the console asks for before the first release, with the answers below.

TVs are a separate form factor: under **Test and release › Advanced settings › Form factors**, add Android TV only when the TV app should go through Play's TV review too.

## Target API level

`targetSdk` is 37, above what Play requires of new apps and updates.

## App access

Kivo needs a Kivo server to sign in to, so reviewers need one. Give them, under **App content › App access**, the address of a demo server reachable from the internet and a username and password for a viewer account on it, with a sentence saying the app is a client for a self-hosted server. Without a server, the app stops at sign-in and review can reject it.

## Permissions

| Permission | Why |
| --- | --- |
| `INTERNET`, `ACCESS_NETWORK_STATE` | Reaching the user's own server, and the address that answers when the network changes |
| `ACCESS_LOCAL_NETWORK` | Finding and reaching a server on the home network; asked for at sign-in on Android 17 and later |
| `POST_NOTIFICATIONS` | The playback notification, and download progress. Asked for as the first download starts; playback works without it |
| `CAMERA` | Link a device, scanning the QR code a TV shows to sign it in. Asked for as the scanner opens; the frames are read on the phone and never stored or sent. Typing the code works without it |
| `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_MEDIA_PLAYBACK`, `WAKE_LOCK` | Playback that carries on with the screen off or the app in the background |
| `FOREGROUND_SERVICE_DATA_SYNC` | Downloads the user started, coming in with a progress notification |

`POST_NOTIFICATIONS` and `CAMERA` need no Play declaration form; the table is for the store listing and review notes.

## Foreground service declaration

Under **App content › Foreground service permissions**, declare both types the app uses, each with a short video of it in use (an unlisted YouTube link is fine):

- **Media playback** (`mediaPlayback`, `PlaybackService`): "Plays the user's audiobooks and videos from their own server, including in picture in picture, with the screen off or another app open. The user starts and stops playback; the media notification controls it."
- **Data sync** (`dataSync`, WorkManager's `SystemForegroundService`): "Downloads the videos and audiobooks the user chooses from their own server for watching offline, with a progress notification and a pause button. It runs only while downloads the user asked for are coming."

## Data safety

The app sends data only to the server the user signs in to, which the user runs. Nothing goes to the developer, and there are no ads or analytics.

- Does the app collect or share user data? **No**, for the app's own code: account details, watch history and playback reports go to the user's own server, not to the developer or anyone else.
- Is data encrypted in transit? Only when the user's server uses HTTPS; home servers are often plain HTTP on the local network. Answer **No** if the form forces a single answer for all traffic.
- Can users ask for their data to be deleted? Their data lives on their own server; deleting their account there deletes it.
- **Google Cast** (Google Play services' Cast SDK, used on phones to cast to a TV): Google discloses what the SDK collects in its [Cast SDK data disclosure](https://developers.google.com/cast/docs/data_disclosure). Copy its current answers into the form, as Play counts what an SDK collects as the app's.

Check the merged manifest has no `com.google.android.gms.permission.AD_ID` before answering the advertising ID question **No**.

## Content rating

Fill in the IARC questionnaire under **App content › Content ratings** with category **Entertainment** (or the closest offered for a media player). The app shows only what the user's own server holds; it has no store, no ads, no chat or user-to-user sharing, no location sharing and no gambling. Answer **No** to the violence, sexual content, language, controlled substance and user interaction questions, and say in any notes field that the app plays the user's own media library.

## Store listing

- App name: Kivo
- Short description (80 characters)
- Full description (4,000 characters)
- App icon, 512 × 512 PNG
- Feature graphic, 1024 × 500 PNG or JPEG
- Phone screenshots: 2 to 8, PNG or JPEG, each side 320 to 3840 px and the long side at most twice the short one. A modern phone's full screen is taller than 2:1, so take them at 1080 × 1920 (`adb shell wm size 1080x1920`, then `adb shell wm size reset`):
  1. Home, with Continue Watching and Recently Added
  2. A library's grid, with the letter rail
  3. A movie's page, with Play, the actions and the overview
  4. A series' page, with its seasons and episodes
  5. The player, in landscape, with the controls showing
  6. Now Playing for an audiobook
  7. Downloads
  8. Search
- Tablet screenshots (optional, for the tablet listing): Home, a library's grid and a work's page at 7 and 10 inches, in landscape.
