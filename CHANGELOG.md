# Changelog

## v0.20.0 - 2026-09-01

### Removed

- Remove `--color-sub`, deprecated in v0.18.0 and warned about since. Passing it
  now fails with `unknown flag: --color-sub` rather than being silently ignored,
  which is the outcome a deprecation cycle exists to produce.
  `--color-by group,cell_subtype` has reproduced it exactly since v0.18.0, for
  every kind of query group and not only `cell_type` ones; under a named family,
  ask for the field directly with `--color blue --color-by cell_subtype`. Three
  behaviors did not survive the move, each of them dropping something that
  carried no meaning: neurons without a `cell_subtype` share the `(empty)`
  group's own tone instead of keeping whatever base tone their position in the
  group happened to give them, a query group holding no root IDs of its own no
  longer reserves a color family and is named in the output, and tones under a
  named family are assigned once across the whole result rather than restarting
  inside every group.

## v0.19.0 - 2026-08-31

### Added

- Choose the field the published labels carry with `--label-by`, on both `add`
  and `state-transfer`. A comma-separated list falls back field by field, so
  `--label-by cell_subtype,cell_type` labels a neuron by its subtype where one
  is annotated and by its cell type otherwise. The default is unchanged:
  `cell_type` falling back to `cell_class`.
- Choose the filterable tag chips with `--label-tags`, which takes the same
  fields and also `none`, publishing labels with no tags at all. The default is
  unchanged: `cell_class`, `cell_instance`, `side`, and `super_class`. Both
  flags accept `super_class`, `cell_class`, `cell_type`, `cell_subtype`,
  `cell_instance`, `side`, `region`, `tract`, `nerve`, `hemilineage`, and
  `proofread`, and complete their lists in the shell, offering only the fields
  a list does not already name.
- Tag each of a neuron's regions separately. `region` is the one multi-select
  field, so a neuron annotated to both LX and LW is tagged `region_lx` and
  `region_lw` and either one filters it. The full annotated set is published
  even when the query matched on a single region, since a neuron shown because
  it matched LX is still in LW.
- Reject an unknown, empty, or repeated field before the query runs, so a typo
  costs a message rather than a scene labeled with blanks.

### Changed

- Describe the labels by the field they carry rather than as cell types, in the
  `--labels` help, the `labels` command, the note printed before publishing,
  and the gist's own description. Passing no `--label-by` still produces
  cell-type labels.
- Postpone the removal of `--color-sub` to v0.20.0, which v0.18.0 had named for
  v0.19.0. The flag still works and still warns, and
  `--color-by group,cell_subtype` remains the replacement.

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
