# Changelog

## v0.18.0 - 2026-08-25

### Added

- Nest `--color-by` two levels deep: two comma-separated fields let the first
  choose each group's color family and the second vary the tone within it, as
  in `--color-by cell_type,cell_subtype`. Two levels need several families, so
  this form requires `--color colored`, which is the default when `--color` is
  omitted; a named family or a hex color warns and colors by the second field
  alone.
- Add `--color-by group`, which colors by the query group a neuron matched
  rather than by any value on its row. It is the one partition no metadata
  field describes: a mixed union draws its groups from different columns, and
  `--intersect` builds a `value1/value2` cross-product. Because the query
  groups are the outer level by construction, `group` can only be the first
  field.
- Add continuous `--color-by pos_x`, `pos_y`, and `pos_z`, which spread the
  matched neurons along a ramp taken from the soma position rather than sorting
  them into groups — viridis under `colored`, dark to light under a named
  family. Neurons whose `position` is empty or malformed take a neutral gray
  and are counted on their own line.
- Add `--color-by root_id` for one color per neuron, reported as a total rather
  than one line per group.
- Complete the new `--color-by` fields in both level positions.

### Changed

- Deprecate `--color-sub`, which still works, warns on use, and will be removed
  in v0.19.0. `--color-by group,cell_subtype` is the exact replacement under
  `--color colored`; for a named family, ask for the field directly with
  `--color blue --color-by cell_subtype`. Three behaviors do not survive the
  move, each of them dropping something that carried no meaning: neurons
  without a `cell_subtype` now share the `(empty)` group's own tone instead of
  keeping whatever base tone their position in the group happened to give them,
  a query group holding no root IDs of its own no longer reserves a color
  family and is now named in the output, and tones under a named family are
  assigned once across the whole result rather than restarting inside every
  group.
- Report Linux X11 clipboard read failures instead of reading them as an empty
  clipboard, and return immediately when no application owns the selection
  rather than blocking for the full read deadline.

### Fixed

- Publish a release only after every binary, signature, installer, and
  `checksums.txt` is uploaded. `install.sh` and `install.ps1` resolve
  `releases/latest`, so an install started during a release run could fail for
  minutes with `curl: (22) The requested URL returned error: 404`; it now gets
  the previous complete release until the new one is ready.

### Security

- Build releases with Go 1.26.7. Go 1.25 reached the end of its support window,
  and 1.26 is the last line that still runs on macOS 12 Monterey.

## v0.17.2 - 2026-08-20

### Fixed

- Retry transient GitHub release-asset downloads in the Unix and Windows
  installers (connection resets, truncated transfers, 503s) instead of
  failing on the first attempt.

### Security

- Build releases with Go 1.25.14.

## v0.17.1 - 2026-07-31

### Fixed

- Authenticate release installers and binaries with the production-ready
  Sigstore Go verifier built into crantcli, removing the external Cosign
  dependency from updates. Transitional legacy bundles remain published so
  v0.17.0 clients can still reach this release.

## v0.17.0 - 2026-07-31

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
