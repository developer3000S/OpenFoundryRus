# WorldView — a Cesium build on OpenFoundry

This branch (`worldview` on [mdc159/OpenFoundry](https://github.com/mdc159/OpenFoundry))
is a personal build that uses [DioCrafts/OpenFoundry](https://github.com/DioCrafts/OpenFoundry)
as its data-platform substrate, not as a project to mirror.

It is **not a fork in the "we plan to PR back" sense.** Upstream is chasing
Palantir Foundry parity; this branch is chasing a Cesium-based geospatial
intelligence surface inspired by the public
[WorldView demo](https://www.youtube.com/watch?v=rXvU7bPJ8n4) by
ex-Google Maps PM Balavo. Different vision; same bones underneath.

## What's at `/worldview`

A Cesium 3D globe with two live data layers:

- **OpenSky Network** ADS-B aircraft (~6,000 commercial flights, color-coded
  by altitude band) — uses a server-side OAuth2 client_credentials proxy in
  Vite so credentials never reach the browser bundle.
- **CelesTrak active satellites** (~15,000, color-coded by orbital regime:
  LEO / MEO / GEO / HEO) — TLE positions propagated client-side via
  `satellite.js`, refreshed every 10 s, with a 6-hour in-memory cache on
  the dev server to respect CelesTrak's rate limits.

Plus: stats bar, landmark fly-to buttons (SF / NYC / London / Tokyo /
Singapore / Dubai / Sydney / Tehran), and a click-to-pin detail panel for
both layers.

## Related routes on this branch

| Route | What it is |
|---|---|
| `/worldview` | The Cesium globe — the main attraction |
| `/globe-flights` | The earlier flat-Mercator deck.gl version, kept as fallback |
| `/maplibre-demo` | A MapLibre globe with OSM tiles + city markers (capability validator) |
| `/geospatial` | The upstream OpenFoundry geospatial workspace, with two sample layers seeded by `tools/dev-smoke/geospatial-smoke.sh` |

## How this branch differs from `main`

`main` mirrors `DioCrafts/OpenFoundry`. `worldview` adds:

| Commit | What it does |
|---|---|
| `feat(worldview)` | Cesium 3D globe with OpenSky + CelesTrak |
| `feat(globe-flights)` (×2) | deck.gl flat-Mercator flights + hover/pin panel |
| `chore(geospatial)` | Smoke-test workflow + 3-line defensive fix for `MapFeature.properties` being `omitempty` |
| `chore(local-dev)` (×2) | OAuth proxy retarget, dev compose context paths |
| `local: drop orphan migration scaffold` | Removed `openfoundry-go/` orphan tree that broke `make vet`/`make build`/`make test` |

Nothing here changes the platform's existing services or contracts — every
addition is in `apps/web/` plus one Vite proxy rule. The platform stays
the platform.

## Phase 2 candidates

Roughly ordered by ROI for the WorldView aesthetic:

1. Google Photorealistic 3D Tiles (free Maps Platform key) — buildings + terrain
2. Filter shaders (CRT / NVG / FLIR) via Cesium post-processing
3. IP camera ingestion as the first OpenFoundry-managed dataset → Cesium layer
4. 4D playback / event scrubber
5. Military flight feed (ADSBExchange)
6. Satellite orbit traces (Cesium `PathGraphics`)

## Use what's useful

If anything here is interesting to anyone — upstream maintainers, other
forks, random visitors — feel free to cherry-pick. The relevant work
mostly lives in:

- `apps/web/src/routes/worldview/` — the Cesium build
- `apps/web/src/routes/globe-flights/` — the deck.gl fallback
- `apps/web/vite.config.ts` — the OpenSky + CelesTrak server-side proxies
- `tools/dev-smoke/geospatial-smoke.sh` — the `/api/v1/geospatial`
  endpoint exercise script

License inherited from upstream OpenFoundry.
