// Package version is Kivo's version: the release the repository is at,
// the version a build reports, and how two versions compare.
//
// Versions are semantic versions, such as 0.4.0. A build that isn't a
// release reports the patch after the latest release as a prerelease, with
// its commit as build metadata, such as 0.3.1-dev+56c0018d: it holds work
// since 0.3.0, so it sorts after 0.3.0 and before any later release.
package version
