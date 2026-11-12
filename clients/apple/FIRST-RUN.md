# The iPhone app: building and running it on a Mac

Linking the Kotlin core and building the Swift app needs a Mac with macOS 15
or later and Xcode 26 or later.

## 1. Install

Install Xcode from the App Store, then once:

```sh
sudo xcode-select -s /Applications/Xcode.app
sudo xcodebuild -license accept
xcodebuild -runFirstLaunch
xcodebuild -downloadPlatform iOS
```

With Homebrew (brew.sh), install JDK 21 and XcodeGen 2.46.0 or later:

```sh
brew install openjdk@21 xcodegen
sudo ln -sfn /opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk /Library/Java/JavaVirtualMachines/openjdk-21.jdk
```

Gradle, Kotlin/Native, SKIE and the Swift packages are pinned in the
repository and download themselves.

## 2. Set the signing team

```sh
cd clients/apple
cp Config/Signing.local.xcconfig.example Config/Signing.local.xcconfig
```

Put your Team ID in it (Xcode › Settings › Accounts, or developer.apple.com
under Membership details). The file isn't committed. If the bundle
identifier `com.kivo.player` is taken for your team, set `KIVO_BUNDLE_ID` in
the same file. A free Apple ID works, but the app expires after 7 days.

## 3. Build the core and generate the project

```sh
./scripts/build-core.sh   # KivoUI's tokens and Frameworks/KivoCore.xcframework
xcodegen generate         # writes Kivo.xcodeproj and Kivo/Info.plist
open Kivo.xcodeproj
```

The first `build-core.sh` downloads about 1 GB and takes a few minutes.
After that Xcode runs it before every build. Run `xcodegen generate` again
after adding or removing a Swift file or changing `project.yml`; never edit
the `.xcodeproj`. If Xcode keeps compiling against an old core ("has no
member …" after the core's API changed), delete `clients/core/build` and
DerivedData and build again.

## 4. Run in the Simulator

Pick the **Kivo** scheme and an iPhone Simulator, and Run (⌘R). Allow local
network access when macOS asks. Sign-in lists the servers it finds on the
network; if yours isn't listed, type its address under Enter address.

## 5. Run on an iPhone

1. On the phone, turn on Settings › Privacy & Security › Developer Mode. It
   shows after the phone has been connected to a Mac with Xcode once.
2. Connect it, unlock it, tap Trust, pick it as the run destination in
   Xcode, and Run.
3. With a free Apple ID, trust the developer once: Settings › General › VPN &
   Device Management.
4. Allow **Local Network** when asked, or the app finds no server on the
   LAN.

Over SSH, unlock the keychain before building for a phone:
`security unlock-keychain ~/Library/Keychains/login.keychain-db`.

## 6. Command-line builds and tests

From `clients/apple`:

```sh
./scripts/build-core.sh && xcodegen generate
xcodebuild -project Kivo.xcodeproj -scheme Kivo \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build/DerivedData build
xcrun simctl boot 'iPhone 17'; open -a Simulator
xcrun simctl install booted build/DerivedData/Build/Products/Debug-iphonesimulator/Kivo.app
xcrun simctl launch booted com.kivo.player
```

The unit tests:

```sh
xcodebuild -project Kivo.xcodeproj -scheme Kivo \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build/DerivedData CODE_SIGNING_ALLOWED=NO \
  test -only-testing:KivoTests
```

### Pointing at a server

In a Debug build, `KIVO_SERVER` in the app's environment names the server
sign-in connects to instead of browsing the network. `simctl` passes it with
the `SIMCTL_CHILD_` prefix:

```sh
SIMCTL_CHILD_KIVO_SERVER=http://192.168.1.10:8484 xcrun simctl launch booted com.kivo.player
```

The UI tests (`KivoUITests`) launch the app with it. `xcodebuild test`
passes variables prefixed `TEST_RUNNER_` to the tests without the prefix:

```sh
export TEST_RUNNER_KIVO_SERVER=http://192.168.1.10:8484
export TEST_RUNNER_KIVO_USERNAME=…
export TEST_RUNNER_KIVO_PASSWORD=…
export TEST_RUNNER_KIVO_LIBRARY=Movies   # optional
xcrun simctl uninstall booted com.kivo.player   # start signed out
xcodebuild -project Kivo.xcodeproj -scheme Kivo \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build/DerivedData -resultBundlePath build/uitest.xcresult \
  test -only-testing:KivoUITests
```

Keep the password out of the shell history and the repository. Each test
names the other variables it reads at the top of its file.

### The download test

`scripts/download-test/server.sh` builds a server from the checkout,
generates small test media and serves it on `127.0.0.1:18490` behind a proxy
that slows and interrupts downloads; `server.sh stop` stops it. The account
is `test` with the password `download test`. `DownloadTests` stops the server,
so run each of its tests alone on a fresh `server.sh` and a freshly
installed app:

```sh
scripts/download-test/server.sh
export TEST_RUNNER_KIVO_DOWNLOAD_SERVER=http://127.0.0.1:18490
export TEST_RUNNER_KIVO_DOWNLOAD_USERNAME=test
export TEST_RUNNER_KIVO_DOWNLOAD_PASSWORD='download test'
xcrun simctl uninstall booted com.kivo.player
xcodebuild -project Kivo.xcodeproj -scheme Kivo \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build/DerivedData -only-testing:KivoUITests/DownloadTests test
scripts/download-test/server.sh stop
```

The other UI tests run against the same server with
`TEST_RUNNER_KIVO_SERVER=http://127.0.0.1:18490`, the same account, and
`TEST_RUNNER_KIVO_LIBRARY='Home Videos'`.
