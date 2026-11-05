# The iPhone app: building and running it on a Mac

The app's Kotlin compiles on Linux too (CI type-checks the core for iOS and
tvOS), but linking the core and everything Swift needs a Mac. It builds with
Xcode 26.3 on macOS 15.7, Kotlin 2.4.0 and SKIE 0.10.15, without warnings,
and runs in the iOS 26 Simulator against a real server. This is how to set
up the Mac, build, run it in the Simulator and on an iPhone, and run the
UI tests.

## 1. Install

- **macOS 15 or later**, and **Xcode 26 or later** from the App Store (the
  iPhone 17 Simulator comes with Xcode 26). Then, once:

  ```sh
  sudo xcode-select -s /Applications/Xcode.app
  sudo xcodebuild -license accept
  xcodebuild -runFirstLaunch
  xcodebuild -downloadPlatform iOS
  ```

- **Homebrew** (brew.sh), then **JDK 21** and **XcodeGen**:

  ```sh
  brew install openjdk@21 xcodegen
  sudo ln -sfn /opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk /Library/Java/JavaVirtualMachines/openjdk-21.jdk
  /usr/libexec/java_home -v 21     # prints the JDK's path
  xcodegen --version               # 2.46.0 or later; project.yml asks for 2.46.0
  ```

Gradle 9.8.1, Kotlin 2.4.0 with its Kotlin/Native toolchain, SKIE 0.10.15,
Nuke 13.2.0, the Google Cast SDK 4.8.6 (Google's Swift package) and libass 0.17.5
(MPVKit's prebuilt libass-build, with FreeType, HarfBuzz and FriBidi) are pinned
in the repository and download themselves.

## 2. Set the signing team

```sh
cd clients/apple
cp Config/Signing.local.xcconfig.example Config/Signing.local.xcconfig
```

Put your Team ID in it. It isn't committed. The Team ID is in Xcode ›
Settings › Accounts (select the team: a free Apple ID is a "Personal Team"),
or on developer.apple.com under Membership details for a paid one. From
Terminal, once Xcode has signed in:

```sh
defaults read com.apple.dt.Xcode IDEProvisioningTeamByIdentifier | grep -E 'teamID|teamName'
```

The ten characters in parentheses after "Apple Development: Your Name" in
`security find-identity -v -p codesigning` are the certificate's ID, not the
team's. A free Apple ID works for the Simulator and for a phone, but the app
expires after 7 days. If the bundle
identifier `com.kivo.player` is taken for your team, set `KIVO_BUNDLE_ID`
in the same file.

## 3. Build the core and generate the project

```sh
./scripts/build-core.sh   # Gradle: KivoUI's tokens and Frameworks/KivoCore.xcframework (Debug)
xcodegen generate         # writes Kivo.xcodeproj and Kivo/Info.plist
open Kivo.xcodeproj
```

The first `build-core.sh` downloads Gradle and the Kotlin/Native toolchain
(about 1 GB, into `~/.konan`) and links four frameworks: about five minutes
on an M-series Mac. After that Xcode runs it before every build (the
`KivoCore` target), and it takes seconds unless the core changed. The same
by hand, from `clients/`: `./gradlew :tokens:swiftTokens
:core:assembleKivoCoreDebugXCFramework` (or `Release`), then copy
`core/build/XCFrameworks/debug/KivoCore.xcframework` to `apple/Frameworks/`.
The script finds JDK 21 by `java_home` or Homebrew's path when Xcode runs it
without `JAVA_HOME`. When the framework changed, it stamps every file in
`Frameworks/KivoCore.xcframework` with the current time, since Xcode judges
by timestamps whether to process it again, and copied with Gradle's own it
could keep compiling against the old module ("AppleServer has no member …"
after the core's API changed). If that error shows anyway, delete
`clients/core/build` and the DerivedData folder and build again.

Run `xcodegen generate` again after adding or removing a Swift file, or
changing `project.yml`; never edit the `.xcodeproj`.

## 4. Run in the Simulator

Pick the **Kivo** scheme and **iPhone 17**, and Run (⌘R). The Simulator uses
the Mac's network, so it reaches the server as the Mac does, VPN included.

1. Sign-in lists the servers Bonjour finds on the network (the server
   advertises `_kivo._tcp`). One server, with none signed in to before, goes
   straight to its password. If macOS asks whether the Simulator may find
   devices on the local network, allow it. If the server isn't found, type
   its address under Enter address: `192.168.1.10` becomes
   `http://192.168.1.10:8484`.
2. Sign in with a Kivo username and password. iOS then offers to save the
   password; either answer is fine.
3. The app reopens signed in. Deleting it forgets the server, so it starts
   at sign-in again.

## 5. Run on the iPhone

1. On the phone: Settings › Privacy & Security › Developer Mode, on (it
   restarts). It shows only after the phone has been connected to a Mac
   with Xcode once.
2. Connect it by cable, unlock it and tap Trust. In Xcode, pick it as the run
   destination (Window › Devices and Simulators lists it once it's paired;
   later it works over Wi-Fi), and Run. Xcode makes the provisioning profile
   for your team the first time.
3. With a free Apple ID, the first launch is refused until you trust the
   developer: Settings › General › VPN & Device Management › your Apple ID ›
   Trust. Rebuild every 7 days.
4. Allow **Local Network** when asked, as sign-in opens: without it Bonjour
   finds nothing and nothing on the LAN answers. Refused, sign-in says so
   with Open Settings (Settings › Privacy & Security › Local Network ›
   Kivo); a typed address, or Scan server QR code, still works for a server
   reached another way.

Over SSH, building for a phone signs with your certificate: unlock the login
keychain first (`security unlock-keychain ~/Library/Keychains/login.keychain-db`).

### The device name

Since iOS 16 `UIDevice.current.name` is the model ("iPhone") unless the app
has the user-assigned device name entitlement
(`com.apple.developer.device-information.user-assigned-device-name`), so a
session is named after the device's model, "iPhone 17 Pro" from the machine
identifier (`AppleIdentity.modelName`).

## 6. What the UI test covers

Checked in the Simulator by the UI test, against a test server: discovery, a wrong
password ("Wrong username or password."), signing in, reopening signed in,
Home's rows with artwork, the Libraries menu and a library's grid with its
chips, sort menu, long press menu and letter scrubber, a movie's page, a
series with its season menu and an episode row's swipe action, an episode's
page, Downloads, a Settings choice, and playing a movie from its resume
position: the controls hiding 4 seconds after the last touch and staying
while tapped, paused or with a sheet open, choosing a subtitle and switching
Dialogue boost in Audio & Subtitles, Audio & subtitles, Speed and Stream info
in Playback Settings, the scrub bar's bubble, a double tap's skip, and
close, which stops the play on the server (admin Now playing shows it
while it plays).

## 7. Building from the command line, or over SSH

Everything above also works from Terminal, or over SSH from another
machine, with command-line builds, the UI tests and Simulator screenshots.

1. For SSH: System Settings › General › Sharing › **Remote Login**, on, for
   your user, with your public key in `~/.ssh/authorized_keys`.
2. A non-interactive SSH shell doesn't read `~/.zprofile`, where Homebrew's
   setup usually is: run commands through a login shell
   (`ssh mac 'zsh -lc "…"'`), or put it in `~/.zshenv`:

   ```sh
   eval "$(/opt/homebrew/bin/brew shellenv)"
   export JAVA_HOME="$(/usr/libexec/java_home -v 21)"
   ```

3. Stay logged in to the Mac's desktop (the Simulator needs the session), and
   keep it awake (`caffeinate -dis &`, or Energy settings).

To build from a copy rather than a checkout, sync the tree without `.git`,
`build/`, `.gradle/` and the generated files (`Frameworks/`,
`Kivo.xcodeproj`, `Kivo/Info.plist`, `KivoUI/Generated/`), so Gradle's and
Xcode's caches stay warm between syncs.

The commands, from `clients/apple` (`xcrun simctl list devices available`
names the simulators; a UDID instead of a name avoids touching a simulator
someone else has booted):

```sh
./scripts/build-core.sh && xcodegen generate
xcodebuild -project Kivo.xcodeproj -scheme Kivo \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build/DerivedData build
xcrun simctl boot 'iPhone 17' 2>/dev/null; open -a Simulator
xcrun simctl install booted build/DerivedData/Build/Products/Debug-iphonesimulator/Kivo.app
xcrun simctl launch booted com.kivo.player
xcrun simctl io booted screenshot /tmp/kivo.png
xcrun simctl spawn booted log stream --level debug --predicate 'process == "Kivo"'
```

`xcodebuild ... build 2>&1 | grep -E 'error:|warning:'` gives the compiler's
findings alone.

### The UI test

`KivoUITests` drives the app against a real server: discovery or the typed
address, a wrong password and then the right one, Home, a library chosen
from the Libraries menu and the screens of section 6, and in
`testPlayerControls` the player's controls and sheets.
`testDialogueBoostKeepsPlaying` resumes a video 10:38 in (`KIVO_BOOST_VIDEO`
in `KIVO_LIBRARY`, Night Shift by default), switches Dialogue boost on,
plays it again after a relaunch with the boost kept on, and closes the
player while paused. `BookPlayerTests`
streams a book from the audiobooks library (`KIVO_BOOKS`, Audiobooks by
default; `KIVO_BOOK` names one by its title, else the first), keeps playing
on Home with the mini player, and in Now Playing changes the speed and sets
the account's back to 1×, jumps from the chapters, sets and turns off a
sleep timer, goes to the next chapter, shows the light theme, and stops.
`SearchTests` goes to Search, whose field takes the keyboard over the
placeholder, types "name of the rose" (`KIVO_SEARCH`), opens a result and
comes Back to the same query and results without the keyboard, keeps them
across another tab, and taps Search on a result's page to go back to the
results with the field focused; its screenshots are named `ios-search-…`. It reads the server and the account from the environment;
`xcodebuild test` passes a variable prefixed `TEST_RUNNER_` into the test
runner without the prefix. Keep the password out of the shell's history and
the repository, for example in a file only you can read:

```sh
# ~/.kivo-uitest-env, chmod 600:
#   export TEST_RUNNER_KIVO_SERVER=http://192.168.1.10:8484
#   export TEST_RUNNER_KIVO_USERNAME=…
#   export TEST_RUNNER_KIVO_PASSWORD=…
#   export TEST_RUNNER_KIVO_LIBRARY=Movies   # optional; Movies by default
source ~/.kivo-uitest-env
xcrun simctl uninstall booted com.kivo.player   # to start signed out
xcodebuild -project Kivo.xcodeproj -scheme Kivo \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build/DerivedData -resultBundlePath build/uitest.xcresult test
xcrun xcresulttool export attachments --path build/uitest.xcresult --output-path /tmp/kivo-shots
```

Each step's screenshot is an attachment; `manifest.json` in the output
names them. The tests launch the app with `KIVO_CLIENT_ID=kivo-ui-tests`,
which a Debug build reads, so it signs in as that client whatever the install: each run's sign-in
replaces the last run's session, and the account's Where you're signed in
keeps one entry for the tests.

`testLibraryScrollPerformance` flings down the library's grid, five times over,
and measures the scrolling's hitch time ratio and the app's CPU. Run it
alone, on a freshly installed app so the image caches start empty, with
`-only-testing:KivoUITests/SignInAndPlayTests/testLibraryScrollPerformance`.
The Simulator reports only the scroll's duration, not its hitches, which
take a device; neither the Animation Hitches nor the SwiftUI instrument
records in the Simulator, while Time Profiler does. It plays real media, so it leaves the account's resume position
on that movie moved on by a few seconds.

Gotchas found writing it:

- Bonjour goes straight to a server's password when it finds one, which
  may be another server on the network: unless the password screen names
  `KIVO_SERVER`, the test goes back with Change server and types its
  address.
- iOS's "Save Password?" sheet after signing in belongs to
  `com.apple.SafariViewService`, not SpringBoard; the test declines it there.
- Views under a full-screen cover stay in the accessibility tree, so "the
  detail page's Play exists" doesn't mean the player closed; the test waits
  for it to be hittable.
- After a tap, the player's controls hide again 3 seconds after the last
  input while playing; the test pauses before reaching for Close.

### The download test

`DownloadTests` downloads a book and a movie at Standard, kills the app part way
through the movie's transfer and relaunches it, then stops the server and
plays both from their files. It runs against a local server rather than
the test server, since it stops it: `scripts/download-test/server.sh` builds Kivo from
the checkout (Go fetches the toolchain `server/go.mod` names), generates
small media under `~/kivo-download-test` (or `$KIVO_DOWNLOAD_TEST_DIR`),
signs up the account `test` with the password `download test`, and puts
`proxy.go` in front of it on port 18490 (`$KIVO_TEST_PORT`; the server
itself is on 18484, `$KIVO_TEST_SERVER_PORT`). The media is a movie with a
subtitle and Many Tracks, with five English audio tracks and eighteen
subtitle tracks, which `TracksPanelTests` opens Audio & Subtitles on, in
Movies, a book of five chapters in a series in Audiobooks, a
three-episode show over two seasons in Shows, and in Home Videos twelve
videos titled across the alphabet, eight of them in two folders, which
are its collections, and Night Shift, fifteen minutes with 5.1 AC-3 audio
and an English subtitle, long enough for `testPlayerControls`
(`KIVO_LIBRARY="Home Videos" KIVO_CONTROLS_VIDEO="Night Shift"`).
The media is made again when the script changes.
The proxy slows download files to 1.5 MB/s so the test can catch a
transfer part way, drops the connection once after 8 MB so it resumes by
range (its log shows the `Range` and `If-Range` headers), and stops the
server on `POST /__stop`.

```sh
scripts/download-test/server.sh
export TEST_RUNNER_KIVO_DOWNLOAD_SERVER=http://127.0.0.1:18490
export TEST_RUNNER_KIVO_DOWNLOAD_USERNAME=test
export TEST_RUNNER_KIVO_DOWNLOAD_PASSWORD='download test'
xcrun simctl uninstall booted com.kivo.player
xcodebuild -project Kivo.xcodeproj -scheme Kivo \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build/DerivedData -resultBundlePath build/dl.xcresult \
  -only-testing:KivoUITests/DownloadTests test
scripts/download-test/server.sh stop
```

Its screenshots are attachments named `ios-dl-…`. The server listens on
127.0.0.1 only, so it isn't advertised; `KIVO_TEST_LISTEN=0.0.0.0` has it
listen on every address, which Bonjour needs (`dns-sd -B _kivo._tcp` then
lists it), but then a signed-in app also reaches it directly at its local
URL, past the proxy, which `SignedInUserTests` relies on.

`testTitlePagePlaysItsDownloadAndDeleting` downloads the book, the movie at
Standard and the show, sets the movie's place at 2:30 through the API (a video
resumes only after 2 minutes), and checks the movie's page: the badge while
it converts and once it's "Downloaded", Stream Instead, then, with the
server stopped, Resume and the Play from Beginning button beside it
playing the download. Then it deletes Season 2, the whole show by a swipe,
and the book and the movie selected in Edit. Its screenshots are named
`ios-dl2-…`. `testAMovieGoesOnAfterTheCellularSettingAndAPause` turns Allow
cellular downloads on while the movie comes, which moves its transfer to
the other background session, then pauses and resumes it from its page,
and waits for it to be ready; the proxy's log shows each going on by
range. Its screenshots are named `ios-dl3-…`. Since the first test stops
the server, run each with `-only-testing` on its own fresh `server.sh` and
a freshly installed app.

The other tests run against the same server too. A movies library gets
collections only from TMDb, and the generated test media match nothing
there, so the library the tests browse is Home Videos, whose folders give
it the Collections tab:

```sh
export TEST_RUNNER_KIVO_SERVER=http://127.0.0.1:18490
export TEST_RUNNER_KIVO_USERNAME=test
export TEST_RUNNER_KIVO_PASSWORD='download test'
export TEST_RUNNER_KIVO_LIBRARY='Home Videos'
export TEST_RUNNER_KIVO_SHOWS=Shows
export TEST_RUNNER_KIVO_OTHER_LIBRARY=Shows
export TEST_RUNNER_KIVO_SEARCH='test book'
```

`DownloadTests` and `SignedInUserTests` stop the server or sign out, so
they run last, each `DownloadTests` test on a fresh `server.sh`.

`LinkDeviceTests` runs against the same server, with the same environment,
and plays the other device through the API. It starts a code as a TV does,
types it at Settings › Link a device in lower case with a space, checks the
question and its line ("Android TV · On your home network · Just now"), and
Allow makes the TV's poll a session; Don't allow makes it `403 denied`; with
`localNetworks` set to leave out loopback (and set back), the line says
"Not on your home network" with the warning. The launch argument
`-KivoScan <text>` is what the scanner reads, since the Simulator has no
camera: a QR code of another server says so and offers no Allow. Then it
signs out, names the device "Test iPhone", taps Sign in with another
device, approves the code it reads by accessibility ID, and finds the
session under that name and the platform. Its screenshots are named
`ios-link-…`.

`SignedInUserTests` runs against the same server, with the same
environment: it signs in, kills the app, and opens it again with the
server unreachable, through the proxy's `POST /__unreachable`, which drops
every connection until `POST /__reachable`. Home's avatar names the user
while the server can't be reached and once it answers again; then the test
signs out, which asks first. Its second test opens Settings, which names
the user, and checks that a wrong current password in Change password says
so and leaves the app signed in. Its screenshots are named `ios-user-…`.
`IPadLayoutTests` runs on an iPad Simulator, in landscape, with Windowed Apps
on (Settings › Multitasking & Gestures, iPadOS 26's default), against the
server and account `SignInAndPlayTests` reads. At full screen it checks the
sidebar's tabs and that it has no Edit, opens Settings from the sidebar's
account, opens a title from a library's tab, plays it, opens Audio & Subtitles
as a popover, closes the player from the toolbar, and changes tab with ⌘1
and ⌘2. It resizes the window with
the system's window controls, the Left tile and then the resize corner, to a
third of the screen, where it finds the iPhone's tab bar with the list of libraries
and Search, and it checks a work's page stays open while the window narrows
and widens again. Its screenshots are named `ipad-…`.
`SearchFocusTests` runs against the same server and environment: it
chooses Search the first time, after each other tab twice (putting the
keyboard away first with the field's Close, as the keyboard covers the tab
bar), and again on a result's page, and checks each time that the field has
the cursor with the keyboard up.
