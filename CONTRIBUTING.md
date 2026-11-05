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

## Licensing of contributions

Kivo is licensed under the GNU Affero General Public License v3.0. By contributing, you license your contribution under the AGPL-3.0, and you agree that it may also be distributed in the app stores as part of the official Kivo apps.
