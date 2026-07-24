# Root IDs and CAVE

A root ID identifies the current connected object in the CAVE chunkedgraph. Proofreading operations can merge or split objects, so the root that represents a neuron can change over time.

## Why `crantcli` also uses supervoxels

CRANT rows can include both:

- a **root ID**, used to display the current object in Neuroglancer;
- a **supervoxel ID**, used as a stable point of reference for finding the current root.

`check-cave` sends the supervoxel ID to CAVE and compares the returned root with the root stored in CRANT.

```text
stored root ───────────────┐
                           ├─ equal   → current
supervoxel → CAVE → root ──┘  differs → stale
```

Rows without a usable supervoxel cannot be verified this way and are reported separately.

## What a stale result means

A stale result means the stored root and the root currently containing the reference supervoxel differ. It does not automatically tell you why the object changed. Use `cave-history` or `root-info` to inspect associated merge and split activity.

```bash
crantcli cave-history ROOT_ID
crantcli root-info ROOT_ID
```

## Refreshing a scene

`check-cave --refresh-state` replaces stale stored roots with their current CAVE roots in a selected segmentation layer. It does not update the CRANT table.

```bash
crantcli check-cave \
  --all \
  --refresh-state \
  --state scene.json \
  --output refreshed.json
```

Keep the original state when a refresh is part of a scientific record or reproducible analysis.

