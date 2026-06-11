# Third-Party Notices

Skadia is licensed under the GNU Affero General Public License v3.0, see
[`LICENSE`](LICENSE).

This file records third-party software, runtime images, and data sources used
by the public Skadia distribution. Detailed inventories are generated from the
current dependency manifests into [`legal/dependency-inventory/`](legal/dependency-inventory/).

Regenerate the inventories after dependency changes:

```sh
SKADIA_FETCH_PYPI=1 scripts/legal/generate-third-party-inventory.sh
```

## Dependency Inventories

| Inventory | Source |
|---|---|
| [`npm-frontend.tsv`](legal/dependency-inventory/npm-frontend.tsv) | `frontend/package-lock.json` |
| [`go-api-modules.tsv`](legal/dependency-inventory/go-api-modules.tsv) | `services/api/go.mod` and `services/api/vendor/modules.txt` |
| [`python-ml-direct.tsv`](legal/dependency-inventory/python-ml-direct.tsv) | `services/ml/requirements.txt` |
| [`container-images.tsv`](legal/dependency-inventory/container-images.tsv) | `docker-compose.yml` and Dockerfiles |
| [`data-sources.tsv`](legal/dependency-inventory/data-sources.tsv) | Curated runtime data/source inventory |

## Bundled And Vendored Software

The API vendors Go modules under `services/api/vendor/`. Their upstream
license, notice, credit, and copying files are retained there and listed in
`go-api-modules.tsv`.

Notable vendored notice files include:

- `github.com/muktihari/fit` — BSD-3-Clause plus the accompanying FIT SDK
  license file retained at `services/api/vendor/github.com/muktihari/fit/LICENSE-FIT-SDK`.
- `github.com/aws/aws-sdk-go-v2` and `github.com/aws/smithy-go` — Apache-2.0
  license and NOTICE files.
- `github.com/prometheus/*` modules — Apache-2.0 license and NOTICE files.
- `github.com/microcosm-cc/bluemonday` — license and credits files.
- `github.com/ringsaturn/tzf` — MIT license, NOTICE file, and additional
  `LICENSE_ANTI_CSDN.md` file retained from upstream.
- `go.yaml.in/yaml/v2`, `gopkg.in/yaml.v2`, and `gopkg.in/yaml.v3` — license
  and NOTICE files.

## Frontend Packages

Frontend packages are listed in `npm-frontend.tsv`. Most packages declare
permissive licenses in `package-lock.json`; non-MIT licenses include Apache-2.0,
BSD variants, ISC, MPL-2.0, OFL-1.1, CC0-1.0, and related permissive forms.

Known npm metadata exceptions:

- `@mapbox/jsonlint-lines-primitives@2.0.2` declares no license in the package
  metadata and ships no license file in the npm package. It is transitive via
  the MapLibre style stack and must be reviewed when upgrading or replacing that
  stack.
- `rw@0.1.4` has no package-lock license field, but the generator records the
  BSD-style license file shipped in the installed npm package.
- `get-closest@0.0.4` has no package-lock license field, but the generator
  records its MIT license from the installed package metadata.

The frontend also uses `@fontsource-variable/inter`, which is licensed under
OFL-1.1.

## ML Service Packages

Direct ML service requirements are listed in `python-ml-direct.tsv`.

Important limitations:

- `services/ml/requirements.txt` currently contains version ranges for several
  packages. The exact transitive Python dependency set is therefore not fixed
  until installation resolves those ranges.
- `granite-tsfm` is installed from a pinned Git commit and is listed separately
  in the inventory as Apache-2.0 based on the source license at that commit.
- `pillow-jxl-plugin` is listed as GPL-3.0-only from its upstream source
  license; review this dependency before changing Skadia's distribution model.
- Scientific Python wheels can bundle native libraries with their own notices;
  preserve wheel-provided license files in binary distributions.

## Runtime Images

Runtime and build images are listed in `container-images.tsv`. The stack uses
images for PostgreSQL/TimescaleDB, Garage, NATS, BRouter, Martin, Ollama,
LiteLLM, Traefik, Node.js, nginx, Python, Go, and Debian.

Image licenses and notices are governed by the image publishers and the packages
inside each image. When publishing built images, retain image-layer notices and
source-offer obligations required by the base image and installed packages.

## Routing, Map, And Data Sources

Skadia uses runtime data sources that are not committed to the repository but
still carry attribution and usage obligations:

- BRouter is used for external routing, RD5 segment handling, and shipped
  routing profiles. The shipped profile files match `abrensch/brouter` `v1.7.8`
  `misc/profiles2` for the local filenames.
- OpenStreetMap-derived data is used for map layers, routing, enrichment, and
  local imports. Preserve OpenStreetMap attribution and ODbL compliance where
  applicable.
- Protomaps PMTiles are used for offline basemap workflows. Preserve the
  Protomaps and OpenStreetMap attribution required by the downloaded data.
- Copernicus GLO-30 DEM tiles are downloaded from the AWS Open Data mirror for
  elevation workflows.
- BRouter RD5 segment files are downloaded from `brouter.de` when operators use
  the segment download workflow.
- Nominatim and Photon are optional external geocoding fallbacks. Operators
  must comply with the configured provider's terms and rate limits.

## Device And Format Metadata

Skadia reads FIT, GPX, and TCX files and uses device/profile metadata derived
from implementation code, FIT protocol definitions, observed product names, and
documented public sources. When adding new device metadata, keep provenance in
the generator/source data instead of editing generated public lists by hand.
