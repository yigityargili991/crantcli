# Changelog

## Unreleased

### Added

- Add built-in native Wayland/X11 clipboard support with external helper
  fallbacks.
- Add XDG desktop portal browser handoff.

### Changed

- Report clipboard and browser delivery independently so one desktop failure
  cannot suppress another. A clipboard failure warns and prints the URL to
  standard output rather than failing the command; only a result that reached no
  destination at all is an error.
- Warn when clipboard state input is unavailable or holds a bare viewer URL
  with no state fragment, then fall back to the default state; a clipboard URL
  whose fragment holds malformed state is an explicit error instead of a silent
  switch to the default.

### Fixed

- Fix `Error parsing state: URI malformed` in the viewer for any scene holding a
  literal `%` — a layer named `100% confidence`, a shader, an annotation. The
  viewer percent-decodes the fragment, so a bare `%` was read as a broken escape
  sequence. `%` is now escaped as `%25`; nothing else is, so every scene without
  a `%` produces a byte-identical URL to before.
- Decode state URLs the way the viewer does, and fall back to reading the
  fragment as raw JSON when it holds a bare `%`. Previously such a URL was
  rejected outright by `url.Parse`, and via `--state` it surfaced as a confusing
  "reading state file: no such file".
- Fix `--open` on Windows and macOS for large scenes. Both passed the whole
  state URL as a command-line argument, which exceeds the Windows 32767-character
  limit for any realistic scene; oversized URLs now go through the same private
  redirect file already used on Linux.
- Fix `Error parsing state: Expected property name or '}'` in the viewer for
  every `--open` on Windows. Windows takes a single command line rather than an
  argument vector, so the quotes in a state URL were escaped as `\"` and
  rundll32 handed that escaping to the browser unchanged. Any URL the command
  line cannot carry intact now goes through the redirect file, whatever its
  size.
- Fix Linux clipboard reads returning nothing for selections that offer no text
  target, where the external helper fallback was never tried.

## v0.16.2 - 2026-07-28

### Added

- Add crantcli update for securely updating an installed binary to the latest
  compatible release.
- Refresh the built-in Neuroglancer scene and add project status badges.

### Changed

- Improve documentation, site loading, and release installation guidance.

### Security

- Require verified, keylessly signed release installers and binaries before an
  update is installed.
- Harden Unix and Windows installers and extend cross-platform installer checks.
