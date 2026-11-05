# Kivo

Open-source streaming for the media you own.

Kivo is a self-hosted media server for movies, TV shows, home videos and audiobooks. Add your media folders and Kivo matches them, downloads artwork and details, and streams to apps on phones, tablets, TVs and the web. It plays the original file when the device supports it and transcodes when it doesn't. No cloud account required.

## Status

Kivo is pre-1.0. The API and settings may change between releases.

## Apps

Available:

- iPhone and iPad
- Android TV and Google TV (also Fire TV)
- Web
- Samsung Smart TV

Coming soon:

- Android phones and tablets
- LG Smart TV
- Apple TV
- VR

## Server

The server runs in Docker on Linux. Native Windows builds are in progress ([#9](https://github.com/kivodev/kivo/issues/9)), see [running on Windows](docs/windows.md). Native Linux builds are coming soon.

```sh
docker run -d --name kivo \
  --network host \
  -v /path/to/config:/config \
  -v /path/to/data:/data \
  -v /path/to/cache:/cache \
  -v /path/to/media:/media:ro \
  --device /dev/dri \
  ghcr.io/kivodev/kivo:latest
```

Then open `http://<server>:8484` to create an admin account and add libraries.

- Keep `/data` on a local disk, not a network share.
- `--device /dev/dri` enables hardware transcoding on Intel and AMD GPUs. Without it, Kivo transcodes on the CPU.
- Host networking lets the apps discover the server on your network.
- Once a day Kivo asks GitHub for its latest release, sending nothing about your server, so admins see when a new one is out. Set `KIVO_UPDATE_CHECK=false` (or `update_check = false` in `server.toml`) to turn it off.

See also: [reverse proxy setup](docs/reverse-proxy.md), [resetting a password](docs/reset-password.md).

## Learn more

- [Architecture](ARCHITECTURE.md)
- [Contributing](CONTRIBUTING.md)

## License

Kivo is licensed under the [GNU Affero General Public License v3.0](LICENSE).

The Kivo name and logo aren't covered by the code license. Please don't use them in a way that makes a fork look like the official apps.
