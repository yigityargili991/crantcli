import argparse
import gzip
import json
import os
import re
import sys
import traceback
import urllib.error
import urllib.parse
import urllib.request


BEARER_TOKEN_RE = re.compile(r"(?i)Bearer\s+[A-Za-z0-9._~+/=-]+")
TOKEN_FIELD_RE = re.compile(
    r'(?i)("?(?:authorization|token|access[_-]?token|api[_-]?token|refresh[_-]?token|id[_-]?token|auth[_-]?token|bearer[_-]?token|cave[_-]?token)"?\s*[:=]\s*)("[^"]*"|[^\s,}&]+)'
)


def redact(value):
    if value is None:
        return ""
    text = str(value)
    text = BEARER_TOKEN_RE.sub("Bearer [REDACTED]", text)
    return TOKEN_FIELD_RE.sub(r'\1"[REDACTED]"', text)


def emit_ok(skeleton):
    print(json.dumps({"ok": True, "skeleton": skeleton}, separators=(",", ":")))


def emit_error(code, message, details=""):
    print(
        json.dumps(
            {
                "ok": False,
                "error": {"code": code, "message": redact(message), "details": redact(details)},
            },
            separators=(",", ":"),
        )
    )


def parse_args():
    parser = argparse.ArgumentParser(description="Fetch a CAVE skeletoncache skeleton for crantcli.")
    parser.add_argument("--root-id", required=True)
    parser.add_argument("--server", required=True)
    parser.add_argument("--datastack", required=True)
    return parser.parse_args()


def as_float(value, default=0.0):
    try:
        return float(value)
    except Exception:
        return default


def skeleton_url(server, datastack, root_id):
    server = server.rstrip("/")
    datastack = urllib.parse.quote(datastack, safe="")
    return f"{server}/skeletoncache/api/v1/{datastack}/async/get_skeleton/4/{root_id}/flatdict?verbose_level=0"


def fetch_skeleton_dict(url, token):
    request = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})
    with urllib.request.urlopen(request, timeout=180) as response:
        body = response.read()
    return json.loads(gzip.decompress(body).decode("utf-8"))


def l2_ids_by_vertex(data, vertex_count):
    lvl2_ids = data.get("lvl2_ids") or []
    mesh_to_skel_map = data.get("mesh_to_skel_map") or []
    if len(lvl2_ids) == vertex_count:
        return [int(v) if v is not None else 0 for v in lvl2_ids]

    out = [0] * vertex_count
    for idx, skel_idx in enumerate(mesh_to_skel_map):
        if idx >= len(lvl2_ids):
            break
        try:
            skel_idx = int(skel_idx)
        except Exception:
            continue
        if 0 <= skel_idx < vertex_count and out[skel_idx] == 0:
            out[skel_idx] = int(lvl2_ids[idx])
    return out


def skeleton_to_json(root_id, data):
    vertices = data.get("vertices") or []
    edges = data.get("edges") or []
    radius = data.get("radius") or []
    l2_ids = l2_ids_by_vertex(data, len(vertices))

    nodes = []
    for idx, xyz in enumerate(vertices):
        node = {
            "id": idx,
            "x": as_float(xyz[0]) if len(xyz) > 0 else 0.0,
            "y": as_float(xyz[1]) if len(xyz) > 1 else 0.0,
            "z": as_float(xyz[2]) if len(xyz) > 2 else 0.0,
        }
        if idx < len(radius):
            node["radius"] = as_float(radius[idx])
        if idx < len(l2_ids) and l2_ids[idx] != 0:
            node["l2_id"] = l2_ids[idx]
        nodes.append(node)

    clean_edges = []
    for edge in edges:
        if len(edge) < 2:
            continue
        clean_edges.append({"from": int(edge[0]), "to": int(edge[1])})

    return {
        "root_id": str(root_id),
        "source": "cave_skeletoncache",
        "nodes": nodes,
        "edges": clean_edges,
    }


def main():
    args = parse_args()
    token = os.environ.get("CAVE_TOKEN", "").strip()
    if not token:
        emit_error("missing_token", "CAVE_TOKEN is not set")
        return 2

    try:
        root_id = int(args.root_id)
    except ValueError:
        emit_error("invalid_root_id", f"invalid root_id {args.root_id!r}")
        return 2

    url = skeleton_url(args.server, args.datastack, root_id)
    try:
        data = fetch_skeleton_dict(url, token)
        skeleton = skeleton_to_json(root_id, data)
        if not skeleton["nodes"]:
            emit_error("empty_skeleton", f"skeletoncache returned no nodes for root_id {root_id}")
            return 3
        emit_ok(skeleton)
        return 0
    except urllib.error.HTTPError as exc:
        details = exc.read().decode("utf-8", errors="replace")
        if exc.code == 401:
            emit_error(
                "auth_failed",
                "CAVE token is invalid or expired",
                "run 'crantcli setup' or set CAVE_TOKEN with a current token",
            )
        elif exc.code == 400 and "invalid_table_id" in details:
            emit_error(
                "datastack_not_found",
                f"CAVE skeleton table {args.datastack!r} was not found on {args.server}",
                details,
            )
        else:
            emit_error("skeletoncache_failed", f"HTTP {exc.code} from CAVE skeletoncache", details)
        return 1
    except Exception as exc:
        emit_error("skeletoncache_failed", str(exc), traceback.format_exc())
        return 1


if __name__ == "__main__":
    sys.exit(main())
