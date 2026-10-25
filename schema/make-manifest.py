#!/usr/bin/env python3
"""Write the release manifest consumed by the site.

Usage: make-manifest.py REPO VERSION SCHEMA_JSON DIST_DIR
"""
import hashlib
import json
import os
import sys

TARGETS = ("linux-amd64", "linux-arm64", "windows-amd64")


def main() -> int:
    if len(sys.argv) != 5:
        print(__doc__, file=sys.stderr)
        return 2

    repo, version, schema_path, dist = sys.argv[1:]
    with open(schema_path, encoding="utf-8") as f:
        schema_version = json.load(f)["schema_version"]

    targets = {}
    for name in TARGETS:
        binary = "bot.exe" if name.startswith("windows") else "bot"
        path = os.path.join(dist, name, binary)
        with open(path, "rb") as f:
            data = f.read()
        targets[name] = {
            "url": f"https://raw.githubusercontent.com/{repo}/dist/{name}/{binary}",
            "sha256": hashlib.sha256(data).hexdigest(),
            "size": len(data),
        }

    manifest = {
        "version": version,
        "schema_version": schema_version,
        "targets": targets,
    }
    out = os.path.join(dist, "latest.json")
    with open(out, "w", encoding="utf-8") as f:
        json.dump(manifest, f, indent=2)
        f.write("\n")
    print(json.dumps(manifest, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
