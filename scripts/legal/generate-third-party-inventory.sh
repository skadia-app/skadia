#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${1:-$ROOT/legal/dependency-inventory}"
mkdir -p "$OUT"
cd "$ROOT"

node <<'NODE' > "$OUT/npm-frontend.tsv"
const fs = require('fs')
const path = require('path')

const root = process.cwd()
const lockPath = path.join(root, 'frontend/package-lock.json')
const lock = JSON.parse(fs.readFileSync(lockPath, 'utf8'))

function cell(value) {
  if (value === undefined || value === null) return ''
  if (Array.isArray(value)) return value.map(cell).join('; ')
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value).replace(/\t/g, ' ').replace(/\r?\n/g, ' ')
}

function packageName(lockKey, meta) {
  if (meta.name) return meta.name
  const prefix = 'node_modules/'
  const idx = lockKey.lastIndexOf(prefix)
  return idx >= 0 ? lockKey.slice(idx + prefix.length) : lockKey
}

const manualLicenseOverrides = new Map([
  ['rw@0.1.4', ['BSD-style license file', 'known npm package LICENSE']],
  ['get-closest@0.0.4', ['MIT', 'known npm package LICENSE and legacy licenses metadata']],
])

function licenseFromPackageFiles(lockKey, meta) {
  const override = manualLicenseOverrides.get(`${packageName(lockKey, meta)}@${meta.version}`)
  if (override) return override
  const packagePath = path.join(root, 'frontend', lockKey)
  const packageJson = path.join(packagePath, 'package.json')
  if (fs.existsSync(packageJson)) {
    const installed = JSON.parse(fs.readFileSync(packageJson, 'utf8'))
    if (installed.license) return [cell(installed.license), 'installed package.json']
    if (Array.isArray(installed.licenses)) {
      return [installed.licenses.map(item => item.type || item).map(cell).join('; '), 'installed package.json licenses']
    }
  }
  const licenseFiles = fs.existsSync(packagePath)
    ? fs.readdirSync(packagePath).filter(name => /^licen[sc]e/i.test(name))
    : []
  if (licenseFiles.length > 0) {
    const licenseText = fs.readFileSync(path.join(packagePath, licenseFiles[0]), 'utf8')
    if (/MIT License|The MIT License/i.test(licenseText)) return ['MIT', `installed ${licenseFiles[0]}`]
    if (/BSD 3-Clause License|Redistribution and use in source and binary forms/i.test(licenseText)) return ['BSD-style license file', `installed ${licenseFiles[0]}`]
    if (/Apache License/i.test(licenseText)) return ['Apache-2.0', `installed ${licenseFiles[0]}`]
    if (/GNU GENERAL PUBLIC LICENSE[\s\S]*Version 3/i.test(licenseText)) return ['GPL-3.0', `installed ${licenseFiles[0]}`]
    return ['LICENSE file present; review exact terms', `installed ${licenseFiles[0]}`]
  }
  return ['UNKNOWN', 'no package license metadata or local license file']
}

function license(lockKey, meta) {
  if (meta.license) return [cell(meta.license), 'package-lock']
  if (meta.licenses) {
    if (Array.isArray(meta.licenses)) {
      return [meta.licenses.map(item => item.type || item).map(cell).join('; '), 'package-lock licenses']
    }
    return [cell(meta.licenses), 'package-lock licenses']
  }
  return licenseFromPackageFiles(lockKey, meta)
}

console.log(['package', 'version', 'license', 'license_source', 'lock_path', 'resolved'].join('\t'))
for (const [lockKey, meta] of Object.entries(lock.packages || {})) {
  if (!lockKey.startsWith('node_modules/')) continue
  const [lic, licenseSource] = license(lockKey, meta)
  console.log([
    packageName(lockKey, meta),
    cell(meta.version),
    lic,
    licenseSource,
    lockKey,
    cell(meta.resolved),
  ].join('\t'))
}
NODE

python3 <<'PY' > "$OUT/go-api-modules.tsv"
from pathlib import Path

root = Path.cwd()
go_mod = root / "services/api/go.mod"
modules_txt = root / "services/api/vendor/modules.txt"
vendor = root / "services/api/vendor"

direct = set()
in_require = False
for raw in go_mod.read_text().splitlines():
    line = raw.strip()
    if line == "require (":
        in_require = True
        continue
    if in_require and line == ")":
        in_require = False
        continue
    if in_require and line and not line.startswith("//"):
        parts = line.split()
        if len(parts) >= 2 and parts[-2].startswith("v"):
            if "// indirect" not in line:
                direct.add(parts[0])

rows = []
lines = modules_txt.read_text().splitlines()
for i, line in enumerate(lines):
    if not line.startswith("# ") or line.startswith("## "):
        continue
    parts = line[2:].split()
    if len(parts) < 2:
        continue
    module, version = parts[0], parts[1]
    explicit = i + 1 < len(lines) and lines[i + 1].startswith("## explicit")
    module_dir = vendor / module
    notice_files = []
    if module_dir.exists():
        for p in module_dir.rglob("*"):
            if p.is_file() and p.name.lower().startswith(("license", "notice", "credit", "copying")):
                notice_files.append(str(p.relative_to(root)))
    notice_summary = "; ".join(sorted(notice_files)) if notice_files else "-"
    rows.append((module, version, "direct" if module in direct or explicit else "transitive", notice_summary))

print("module\tversion\trelationship\tlicense_notice_files")
for row in rows:
    print("\t".join(cell.replace("\t", " ").replace("\n", " ") for cell in row))
PY

python3 <<'PY' > "$OUT/python-ml-direct.tsv"
from pathlib import Path
import json
import os
import re
import urllib.request

root = Path.cwd()
requirements = root / "services/ml/requirements.txt"
fetch = os.environ.get("SKADIA_FETCH_PYPI") == "1"

def normalize_name(spec: str) -> str:
    if " @ " in spec:
        return spec.split(" @ ", 1)[0].strip()
    name = re.split(r"[<>=!~;\[]", spec, 1)[0].strip()
    return name

def exact_version(spec: str) -> str:
    m = re.search(r"==\s*([A-Za-z0-9_.!+-]+)", spec)
    return m.group(1) if m else ""

LICENSE_OVERRIDES = {
    "fastapi": "MIT",
    "uvicorn": "BSD-3-Clause",
    "asyncpg": "Apache-2.0",
    "nats-py": "Apache-2.0",
    "scipy": "BSD-3-Clause; wheel metadata may include bundled OpenBLAS/GCC runtime notices",
    "numpy": "BSD-3-Clause AND 0BSD AND MIT AND Zlib AND CC0-1.0",
    "shapely": "BSD-3-Clause",
    "scikit-learn": "BSD-3-Clause",
    "httpx": "BSD-3-Clause",
    "aioboto3": "Apache-2.0",
    "neurokit2": "MIT",
    "tsfel": "BSD License classifier; exact BSD variant not declared in PyPI metadata",
    "ruptures": "BSD-2-Clause",
    "sqlglot": "MIT",
    "python-json-logger": "BSD-2-Clause",
    "torch": "BSD-3-Clause",
    "transformers": "Apache-2.0",
    "Pillow": "MIT-CMU",
    "pillow-heif": "BSD-3-Clause; package metadata also lists bundled-license files",
    "pillow-jxl-plugin": "GPL-3.0-only",
    "chronos-forecasting": "Apache-2.0",
}

def license_from_info(name: str, info: dict) -> str:
    if name in LICENSE_OVERRIDES:
        return LICENSE_OVERRIDES[name]
    expression = (info.get("license_expression") or "").strip()
    if expression:
        return expression
    value = (info.get("license") or "").strip().replace("\n", " ")
    classifiers = [c for c in info.get("classifiers") or [] if c.startswith("License ::")]
    if value:
        compact = re.sub(r"\s+", " ", value)
        return compact[:240] + ("..." if len(compact) > 240 else "")
    return "; ".join(classifiers)

def fetch_pypi(name: str, version: str) -> tuple[str, str]:
    if not fetch:
        return "", "not fetched"
    url = f"https://pypi.org/pypi/{name}/{version}/json" if version else f"https://pypi.org/pypi/{name}/json"
    try:
        with urllib.request.urlopen(url, timeout=15) as resp:
            data = json.load(resp)
        return license_from_info(name, data.get("info") or {}), url
    except Exception as exc:
        return "", f"fetch failed: {exc}"

print("package\trequirement\tpinned_version\tlicense_metadata\tsource")
for raw in requirements.read_text().splitlines():
    line = raw.strip()
    if not line or line.startswith("#"):
        continue
    name = normalize_name(line)
    version = exact_version(line)
    if name == "granite-tsfm":
        print("\t".join([
            name,
            line,
            "bd28309b53e7ec6470e71a13e511d3dea8f21123",
            "Apache-2.0",
            "GitHub source LICENSE at pinned commit",
        ]))
        continue
    lic, source = fetch_pypi(name, version)
    if not version:
        source += "; requirement is not fully pinned"
    print("\t".join([
        name,
        line,
        version,
        lic,
        source,
    ]))
PY

python3 <<'PY' > "$OUT/container-images.tsv"
from pathlib import Path
import re

root = Path.cwd()
files = [root / "docker-compose.yml"]
files.extend(root.glob("services/*/Dockerfile"))
files.append(root / "frontend/Dockerfile")

print("source_file\tline\tkind\timage")
for file in sorted(set(files)):
    if not file.exists():
        continue
    for line_no, raw in enumerate(file.read_text().splitlines(), 1):
        stripped = raw.strip()
        if stripped.startswith("image:"):
            image = stripped.split("image:", 1)[1].strip()
            print(f"{file.relative_to(root)}\t{line_no}\tcompose-image\t{image}")
        match = re.match(r"FROM\s+([^\s]+)", stripped)
        if match:
            print(f"{file.relative_to(root)}\t{line_no}\tdockerfile-base\t{match.group(1)}")
        match = re.search(r"COPY\s+--from=([^\s]+)", stripped)
        if match:
            print(f"{file.relative_to(root)}\t{line_no}\tdockerfile-copy-from\t{match.group(1)}")
PY

python3 <<'PY' > "$OUT/data-sources.tsv"
rows = [
    ("source", "purpose", "repo_locations", "notice"),
    ("BRouter", "HTTP routing engine, RD5 segment generation JAR, shipped routing profiles", "config/brouter/profiles; services/api/Dockerfile; docker-compose.yml", "BRouter is MIT licensed. Shipped profiles match abrensch/brouter v1.7.8 misc/profiles2 for the local filenames."),
    ("BRouter segment downloads", "Prebuilt RD5 segment data", "scripts/brouter-download-segments.sh; services/api/internal/osm/brouter_download.go", "Downloaded runtime data is not committed; users should follow the upstream service/data terms."),
    ("OpenStreetMap", "Map data, local OSM import, enrichment, routing graph input", "services/api/internal/osm; services/api/internal/migrate/migrations/000333_map_layers.up.sql; frontend/src", "OSM-derived data requires OpenStreetMap attribution and ODbL compliance where applicable."),
    ("Protomaps PMTiles", "Offline basemap bundle source", "scripts/download-pmtiles.sh; services/api/internal/migrate/migrations/000333_map_layers.up.sql", "Downloaded runtime data is not committed; notices should preserve Protomaps/OpenStreetMap attribution."),
    ("Copernicus GLO-30 DEM", "Elevation data for DEM import and BRouter/HGT conversion", "scripts/dem-download-glo30.sh; scripts/dem-download-all-regions.sh", "Downloaded runtime data is not committed; source is Copernicus GLO-30 via AWS Open Data mirror."),
    ("Nominatim / Photon", "Optional external geocoding fallback", "services/api/internal/migrate/migrations/000300_reverse_geocode_cache_and_fallback_settings.up.sql", "Only used when configured; operators must comply with provider terms and rate limits."),
]
for row in rows:
    print("\t".join(cell.replace("\t", " ").replace("\n", " ") for cell in row))
PY

printf 'Generated third-party inventories in %s\n' "$OUT"
