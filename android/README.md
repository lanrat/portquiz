# portquiz for Android

A minimal Android client for portquiz. It runs the same scanning code as the command line client, compiled into the app with [gomobile](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile).

It needs Android 10 or later on a 64-bit ARM (arm64) device.

## Install

1. Download `portquiz_Android_arm64.apk` from the [latest release](https://github.com/lanrat/portquiz/releases/latest) on the device.
2. Open it and allow installing apps from the browser or file manager when Android asks.
3. On first scan, allow notifications so the scan's progress shows while the app is in the background.

To update, install the newer APK over the old one; settings are kept. A debug build from CI is signed with a different key, so uninstall it before installing a release, and the other way around.

## Use

Enter the portquiz server (a hostname or an IP address) and optionally the ports to test (empty for all ports, or a list like `22,80,1000-2000`), choose TCP, UDP or both, and tap Start. The password must match the server's `-password`; it defaults to `portquiz`, like the command line client, and clearing the field restores the default. Once the scan starts, the status shows the address being tested, and results are listed as ranges of ports with the same result, with open ports highlighted.

Scans keep running with the screen off or the app in the background: progress is shown in a notification, which also has a Stop button. A full scan of one protocol takes at least about 2 minutes. The settings of the last scan are restored the next time the app opens.

The IP version setting chooses between Auto, IPv4, IPv6 and Both. Auto tests every port over both IPv4 and IPv6 when the server's name has an address of each and the phone's network supports both, and otherwise the one version available; Both requires both. Below the results, All, Open and Closed choose which ranges are shown; this only filters the list, so it can be changed after a scan.

When a scan finishes, Summary next to the filter opens a summary of it:

- **Overview:** how many ports were open for each protocol and IP version, and when both IPv4 and IPv6 were tested, the ports open over only one of them.
- **Port map:** a strip for each protocol across the ports tested, colored by result, so blocked ranges stand out. Touch it to see the ports under your finger.
- **How closed ports were closed:** rejected (actively refused), dropped (no response, as when a firewall silently discards traffic) or interfered (something other than the portquiz server answered, such as a proxy or captive portal), with interfered ports listed.
- **Common services:** whether ports such as SSH, HTTPS, DNS, NTP and the usual VPN ports are open.
- **Largest blocked ranges**, and **speed and timing:** how long the scan took, how quickly open ports answered, and the timeout used.

Save JSON saves the scan's report, the same as the command line client's `-json`, and Share sends it as text to another app.

Advanced settings, under Advanced, match the command line client's options and defaults: the timeout in milliseconds and whether it adapts to how quickly open ports respond (`-timeout`, `-adaptive`), connections started per second with 0 for no limit (`-rate`), retries for ports that do not respond, blank for the default (`-retry`), and how many ports are tested at once (`-parallel`). Reset to defaults restores them, and a blank field uses its default.

## Build

Requirements:

- Go (the version in `gomobile/go.mod`)
- JDK 21
- Android SDK with NDK `30.0.16248370` (`sdkmanager "ndk;30.0.16248370"`), and either `ANDROID_HOME` set or `sdk.dir` in `local.properties`

```shell
cd android
./gradlew assembleDebug
adb install app/build/outputs/apk/debug/app-debug.apk
```

The app's version, shown next to its title, is the same as the command line client's: the release tag for releases, otherwise `git describe --tags --always --dirty` as with `make`.

Gradle builds the Go library first with `gomobile/bind.sh`, which builds `gomobile` and `gobind` from the versions pinned in `gomobile/go.mod`. `./gradlew testDebugUnitTest lint` runs the app's tests and checks, and `go test ./...` in `gomobile/` tests the Go adapter.

Layout:

- `gomobile/`: Go module that adapts the `scan` package for gomobile
- `app/`: the Kotlin app; `ScanService` runs scans as a foreground service, `MainActivity` is the main screen and `ReportActivity` the summary of a finished scan

## Release signing

Release builds attached to GitHub releases are signed by the release workflow. Android only installs an update when it is signed with the same key as the installed app, so the key must stay the same for the life of the app. Set it up once:

1. Create a keystore, choosing a strong password when asked:

   ```shell
   keytool -genkeypair -keystore portquiz-release.jks -storetype PKCS12 \
     -alias portquiz -keyalg RSA -keysize 4096 -validity 10000 -dname "CN=portquiz"
   ```

2. Add two repository secrets under Settings → Secrets and variables → Actions:
   - `PORTQUIZ_KEYSTORE_BASE64`: the output of `base64 -w0 portquiz-release.jks` (on macOS: `base64 -i portquiz-release.jks`)
   - `PORTQUIZ_KEYSTORE_PASSWORD`: the keystore password

   Or with the GitHub CLI:

   ```shell
   base64 -w0 portquiz-release.jks | gh secret set PORTQUIZ_KEYSTORE_BASE64
   gh secret set PORTQUIZ_KEYSTORE_PASSWORD
   ```

3. Back up `portquiz-release.jks` and its password somewhere safe, then delete the local copy. If the key is lost, new releases cannot be installed over existing ones; the app has to be uninstalled first.

To build a signed release locally, set `PORTQUIZ_KEYSTORE` to the keystore's path and `PORTQUIZ_KEYSTORE_PASSWORD` to its password, then run `./gradlew assembleRelease -PportquizVersion=v1.2.3`.
