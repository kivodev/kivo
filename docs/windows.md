# Running on Windows

Native Windows builds are in progress ([#9](https://github.com/kivodev/kivo/issues/9)). For now, Windows transcodes on the CPU.

## From the zip

Download `kivo-<version>-windows-amd64.zip` from [Releases](https://github.com/kivodev/kivo/releases) and unzip it anywhere. It holds:

- `kivow.exe`, the server with a tray icon and no console window
- `kivo.exe`, the same server for a terminal, and the admin commands
- `ffmpeg\`, the bundled jellyfin-ffmpeg

Start `kivow.exe`. A Kivo icon appears in the notification area, with Open Kivo, View logs and Quit. Open Kivo goes to `http://localhost:8484`, where you create an admin account and add libraries. Starting `kivow.exe` again while it runs opens Kivo too.

The first time it starts, Windows asks whether to let Kivo through the firewall. Allow it on private networks, so apps on your network can reach it and find it.

To see the log as it runs, quit the tray and run `kivo.exe` in a terminal instead.

## Folders

Kivo keeps everything in `%ProgramData%\Kivo` (usually `C:\ProgramData\Kivo`):

- `config\server.toml`, the optional settings file, with the same keys as in Docker
- `data\`, the database, artwork and subtitles. Keep it on a local disk, not a network drive.
- `data\logs\`, the tray's log, with the previous run's beside it
- `cache\`, everything that can be rebuilt
- `backups\`, the nightly database backups

`KIVO_*` environment variables and `server.toml` override these as they do on Linux.

Kivo runs as you, so it can read your mapped drives and network shares.

## Admin commands

Run them with `kivo.exe` in a terminal, for example:

```powershell
.\kivo.exe reset-password alice
```

See [resetting a password](reset-password.md).
