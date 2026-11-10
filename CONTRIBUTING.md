# Contributing to Kivo

To propose a change, open an issue to talk it over, or open a pull request.

## PR titles

PR titles follow [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): description`, lowercase, no trailing period. The scope is optional.

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`.
- Scopes: `server`, `web`, `core`, `android`, `apple`, `tizen`, `cast`, `docs`, `deps`.

```
fix(apple): one download percent for the whole job
feat(server): Intel OpenCL runtimes for HDR tone mapping
```

Pull requests are squash-merged, so the title becomes the commit on `main` and the release notes.

## Building and testing

Each part's commands are in `server/Makefile`, `server/web/package.json`, the Gradle tasks in `clients/`, [clients/apple/FIRST-RUN.md](clients/apple/FIRST-RUN.md), [clients/tizen/README.md](clients/tizen/README.md) and `clients/chromecast/package.json`.

The server image builds from the repository root:

```
docker build -f server/Dockerfile --build-arg COMMIT=$(git rev-parse --short=8 HEAD) -t kivo .
```

## Versions

The repository has one version, which release-please raises with each release: in `.github/.release-please-manifest.json` and in each file its config's `extra-files` lists, the server's `server/internal/version/version.go` among them. Versions are [semantic versions](https://semver.org/).

- A release build reports the release's version, such as `0.4.0`. The release workflow passes it as the image's `VERSION` build arg, `make build VERSION=…` or `build.ps1 -Version …`.
- Any other server build reports the patch after the latest release as a dev prerelease, with its commit as build metadata, such as `0.3.1-dev+56c0018d` after `0.3.0`. It sorts after the release it's built past and before the next one. `make build` and `go build` find the commit themselves; the image takes it as the `COMMIT` build arg, and is `0.3.1-dev` without one.
- The apps send their version in the `Kivo-App-Version` header of every request: the iPhone and Android apps their version with the build number as build metadata (`0.3.0+78`), the Samsung TV app and the Cast receiver their version, and the web client its server's.

## Licensing of contributions

Kivo is licensed under the GNU Affero General Public License v3.0. By contributing, you license your contribution under the AGPL-3.0, and you agree that it may also be distributed in the app stores as part of the official Kivo apps.
