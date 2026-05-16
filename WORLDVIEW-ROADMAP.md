# WorldView Roadmap

Strategy layer for the [`worldview`](https://github.com/mdc159/OpenFoundry/tree/worldview)
branch. Lists what's been built, what's queued, sequencing rationale,
and the open decisions that haven't been made yet.

Detailed implementation plans for individual features live separately at
`docs/superpowers/plans/YYYY-MM-DD-<feature>.md` — written **only when
that feature is up next**, not maintained speculatively.

---

## Phase 1 — Cesium foundation (shipped)

> **Built:** 2026-05-15. Single working day, evening session.

### What landed

| Surface | URL | Status |
|---|---|---|
| `/worldview` | Cesium 3D globe with ~6k OpenSky aircraft + ~15k CelesTrak satellites, color-coded by altitude / orbital regime, click-to-pin detail panel, 8-landmark fly-to bar | working |
| `/globe-flights` | Flat-Mercator deck.gl + MapLibre version of the live-flights view, kept as fallback | working |
| `/maplibre-demo` | MapLibre globe with OSM raster basemap, 5 city markers (capability validator) | working |
| `/geospatial` | Upstream OpenFoundry geospatial workspace, 2 seeded layers (capitals + continent boxes), full panel UI | working |

### Architecture decisions made

| Decision | Rationale |
|---|---|
| Cesium via `resium` + `vite-plugin-cesium` | Mature React 19 + Vite integration; alternative was raw Cesium with manual worker config |
| Imperative `CustomDataSource` + id-keyed `Map` for ~21k entities | React reconciliation can't keep up at this entity count; diff-in-place keeps the 10s tick fluid |
| Per-layer hooks (`useOpenSkyAircraft`, `useCelestrakSatellites`) | Each data layer is a typed hook returning `{data, lastUpdate, loading, error}` — flipping a layer to an OpenFoundry-managed dataset is a one-file swap, no UI change |
| Server-side OpenSky OAuth in Vite | Credentials never enter the browser bundle; `/external/opensky/*` proxy injects the bearer token per-request |
| Server-side CelesTrak cache in Vite | CelesTrak ToS caps anonymous callers at 1 req/file/hour; 6-hour in-memory cache + warm-from-disk fallback respects them |
| Flat Mercator on `/globe-flights` for deck.gl, globe on `/worldview` for Cesium | deck.gl's `MapboxOverlay` doesn't share projection state with MapLibre globe — kept the working flat version rather than fighting the integration |

### Carryover items (Phase 1 closeout, before Phase 2)

- [ ] **Manual click-to-pin verification on `/worldview`.** Playwright synthetic mouse events don't penetrate Cesium's gesture detector (`isTrusted` filter), so the click pipeline is unverified. A real mouse click on a dot should pop the detail panel. If it doesn't, that's a small bug to fix before going further. Estimated: 5 minutes of poking.

---

## Phase 2 — six candidates, sequenced

Cards listed in recommended build order. Sequencing rationale: visible
ROI first, low-risk before high-risk, dependencies respected,
platform-payoff features in the middle when their value compounds.

### 2.1 — Google Photorealistic 3D Tiles

**Goal.** Replace Cesium's default Bing satellite imagery with Google's
photorealistic 3D mesh tiles (real buildings, real terrain). The single
biggest visual upgrade in Phase 2 — it transforms the globe from "satellite
photo on a sphere" into "fly-through digital twin."

**Scope (in).** Add a Google Maps Platform API key via `.env`. Configure
Cesium to use the Photorealistic 3D Tiles tileset endpoint. Verify the
fly-to landmarks now show real buildings. Add a toggle so the default
Bing imagery can be selected if quota is exhausted.

**Scope (out).** Geofencing tiles to specific cities (free tier covers
the whole planet at moderate volume). Custom shader stylization (Phase 2.3).

**Dependencies.**
- Google Cloud project created
- Map Tiles API enabled
- API key issued + restricted to `localhost:5174` referrers
- `.env` entry: `GOOGLE_MAPS_TILES_KEY=...`
- `.gitignore` already covers `.env`; verify before commit

**Acceptance.**
- Loading `/worldview` shows 3D buildings when zoomed into any of the 8
  landmark cities (verify by flying to NYC, seeing actual Manhattan
  skyline geometry, not just texture)
- Toggle button switches between Photorealistic / Bing imagery
- Quota errors fail gracefully (fall back to Bing, surface a notice)

**Effort.** ~1 hour focused work. Cesium has native support for this
tileset; mostly configuration.

**Risk.**
- Google API quota (free tier: 1,000 sessions/month). For solo eval,
  more than enough. For multi-user, would need a paid plan.
- API key must be referrer-restricted or it leaks via the client.
  Phase 1's server-side proxy pattern doesn't apply here (3D Tiles
  fetches happen in the Cesium runtime, not via our proxy).

**Unlocks.** All subsequent UX feels more immersive — every demo
screenshot looks dramatically better.

---

### 2.2 — Satellite orbit traces

**Goal.** When a user clicks a satellite, draw its full orbital path
around the globe as a faint line. Click another, the previous trace
clears. Makes satellite tracking visually meaningful, not just "yellow
dot in space."

**Scope (in).** Compute the satellite's position every N seconds for
one orbital period (varies: ~90 min for LEO, ~24 hr for GEO). Render as
a Cesium `PolylineGraphics`. Style by orbit regime color.

**Scope (out).** Showing all orbits at once (visual chaos). Historical
ground track (Phase 2.5 / 4D playback territory).

**Dependencies.**
- Phase 1 satellite layer (already shipped)
- `satellite.js` propagation function (already imported)
- Click handler verification from Phase 1 closeout

**Acceptance.**
- Click a satellite → its orbit appears as a translucent line
- Click another → previous orbit clears, new one renders
- Click outside any satellite → all orbit lines clear
- Performance: orbit computation completes in <100 ms even for GEO

**Effort.** ~1.5 hours. Mostly Cesium polyline graphics + propagation math.

**Risk.** Low. `satellite.js` handles the math; Cesium's polyline is
well-trodden API.

**Unlocks.** Sets up the visual vocabulary for any future "trajectory"
feature (aircraft trails, ship wakes, etc).

---

### 2.3 — Filter shaders (CRT / NVG / FLIR)

**Goal.** Add post-processing visual modes — CRT scanlines, night-vision
green, thermal/FLIR pseudo-coloring, plus bloom and sharpen filters.
The "spy thriller aesthetic" from the WorldView reference videos.

**Scope (in).** Cesium has a `Scene.postProcessStages` API for adding
GLSL fragment shaders to the rendered globe. Implement 3 named presets
(CRT, NVG, FLIR) plus 2 effect toggles (bloom, sharpen). Add a small
mode-picker UI bottom-right of the canvas.

**Scope (out).** Custom user-authored shaders (out of scope). Live
parameter sliders (defer; just preset toggles for now).

**Dependencies.** None beyond Phase 1.

**Acceptance.**
- 3 mode buttons + 2 effect toggles render
- Each preset visibly changes the globe's rendering
- Toggles are stackable (e.g., NVG + bloom)
- Filters reset cleanly when "Normal" is selected
- No GPU memory leak after 50+ toggle cycles

**Effort.** ~2 hours. GLSL is the main complexity; Cesium's API is
straightforward.

**Risk.** Medium. Bad shaders can crash the WebGL context. Need to
test on lower-end GPUs (e.g., integrated Intel). Mitigation: keep
shaders short and well-known patterns (chromatic aberration, sepia,
green-channel only).

**Unlocks.** Demo-ability. Every screenshot of WorldView in the
reference videos is in some filter mode.

---

### 2.4 — IP camera ingestion via OpenFoundry connector

**Goal.** The first data layer that flows **through OpenFoundry's
data plane** instead of being fetched directly by the frontend. Take
your existing IP-camera side project (HTTP-accessible feeds with
auth), wire them as a connector in `connector-management-service`,
materialize as a dataset, and consume that dataset from `/worldview`
as a Cesium layer.

This is the inflection point where OpenFoundry actually earns its
keep vs being a standalone Cesium app.

**Scope (in).**
- Use the existing REST/HTTP connector adapter in
  `connector-management-service` (`services/connector-management-service/internal/adapters/rest_api/` and `httpruntime/`)
- Configure it with the camera credentials (IP, login, frame endpoint
  pattern)
- Materialize camera metadata (lat/lon, name, current frame URL) as a
  Postgres-backed dataset via `dataset-versioning-service`
- Add a `useCameraFeed(datasetId)` hook to `apps/web/src/routes/worldview/`
- Render each camera as a Cesium `BillboardGraphics` (or similar) at its
  geo coordinates
- Click a camera → show its latest frame in the detail panel

**Scope (out).** Real-time WebRTC video (defer; static frames on
1-minute interval like WorldView's CCTV demo). Camera control / PTZ
(out of scope; we're consumers). H.264 stream playback (defer).

**Dependencies.**
- `connector-management-service` running (might need to bring it up;
  it's not currently in our running set)
- `dataset-versioning-service` running (same)
- Your camera credentials gathered + safe to put in `.env`
- A decision on whether to fetch frames through Vite proxy
  (recommended) or directly (would require CORS exemption on the cam)

**Acceptance.**
- A new dataset "Mike's Cameras" exists in OpenFoundry with N rows
  (one per camera) including lat/lon/url
- `/worldview` shows N camera icons at the right geographic locations
- Clicking a camera icon shows the current frame in the detail panel
- Frame refresh interval is configurable (default: 60 s)

**Effort.** ~4 hours. Most time is connector configuration + ingestion
plumbing (services we haven't touched yet); the frontend side is small.

**Risk.** Medium-high. We haven't actually run
`connector-management-service` yet — there will be unforeseen
configuration / migration issues. The connector's REST adapter may
or may not match the specific HTTP auth pattern your cams use. Plan
for a discovery spike first.

**Unlocks.** Validates the OpenFoundry-as-substrate thesis. Sets the
pattern for every future data source (military feeds, ship AIS,
earthquake feeds — anything ingestable becomes a dataset). After this,
adding new layers is template work.

---

### 2.5 — Military flight feed (ADSBExchange or alternative)

**Goal.** Add the orange "military planes" overlay from the WorldView
reference videos. Aircraft that don't show on commercial trackers
(MLAT-only, no SQUAWK, military registry).

**Scope (in).**
- Pick a feed source — ADSBExchange is the canonical but requires
  MLAT contributor status or paid API access; alternatives include
  the Air Force Real-Time SQUAWK feed (public, narrower) or community
  scrapes
- Add a server-side fetch + cache layer (similar to CelesTrak)
- Render as a new color-coded layer in `/worldview`
- Filter toggle to show only military / only commercial / both

**Scope (out).** Identifying specific aircraft tail numbers / op
patterns (data analyst work, not engineering work).

**Dependencies.**
- Source feed decided (research spike needed first — 1 hour)
- API key obtained if paid
- Phase 1 OpenSky pattern as the template

**Acceptance.**
- New layer renders ~50-200 military-classified flights
- Visibly distinguishable from commercial (orange vs blue/purple)
- Filter toggle works
- Source attribution + ToS compliance documented

**Effort.** ~1.5–2 hours assuming a viable feed source is found.
Research spike is upfront — if no free source exists, this becomes a
billing decision rather than an engineering task.

**Risk.** High on data access; low on integration. The integration
itself is a copy of the OpenSky pattern.

**Unlocks.** The "panopticon" feel. Plus filtering UX for any future
multi-category layer.

---

### 2.6 — 4D timeline playback

**Goal.** Record the live state of all layers over time and let users
scrub back through history at variable speed. The "Operation Epic
Fury replay" feature from the second reference video — events
correlated against satellite flyovers, airspace closures, etc.

This is the most complex Phase 2 item by far.

**Scope (in).**
- Backend: a service that records the OpenSky + CelesTrak + (future
  cam) state every N seconds into a time-series store (likely
  Cassandra given OpenFoundry already runs it, or a simple
  append-only Iceberg table via `iceberg-catalog-service`)
- Frontend: time-scrubber UI on `/worldview` with play/pause/speed
- Cesium's built-in `Clock` primitive drives the playback
- Entities update positions based on the current playback time,
  not wall-clock

**Scope (out).** Event detection / anomaly correlation (next phase).
Querying recorded data by area / time window (next phase). Storage
budget management (next phase).

**Dependencies.**
- A new backend service (or extension of `ingestion-replication-service`)
- Decision: Cassandra vs Iceberg time-series storage
- Storage budget for X days of position recording
- Phase 2.4 (IP camera ingestion) ideally lands first so the camera
  layer also gets recorded

**Acceptance.**
- Backend records aircraft + satellite state every 30 s for at least
  24 hours, queryable by time range
- Frontend scrubber controls time
- Aircraft entities move backward through recorded positions
- Speed control: 1×, 10×, 100×, 1000×
- Live mode rejoins seamlessly (scrubber to "now")

**Effort.** ~6–8 hours, possibly more. The first real backend work
in this branch.

**Risk.** High. Storage growth (15k satellites × 30s × 1 day = 43M
points/day per layer). Schema design matters. Query latency at
playback scrub speeds may not meet "feels live" target.

**Unlocks.** Everything WorldView's second video does. Also positions
this branch as more than a viewer — it becomes a system of record
for "what was where, when."

---

## Sequencing rationale

```
Phase 1 closeout: click verify  (5 min)
        │
        ▼
2.1 Google 3D Tiles  ──── biggest visual win, low risk
        │
        ▼
2.2 Satellite orbit traces  ──── cheap, satellite layer becomes meaningful
        │
        ▼
2.3 Filter shaders  ──── pure aesthetic, bounded scope
        │
        ▼
2.4 IP camera ingestion  ──── PIVOT: now we're using OpenFoundry as a platform
        │
        ▼
2.5 Military flights  ──── proves the pattern transfers
        │
        ▼
2.6 4D playback  ──── biggest lift, only worth it after 2.4 proves the data plane
```

**Pivot point at 2.4.** Items 2.1–2.3 are all client-side
enhancements. They could be done in a fork that doesn't even use
OpenFoundry. 2.4 is where this branch becomes architecturally
different — the first feature that requires the platform underneath
to work. After 2.4, the value of being "built on OpenFoundry" is
concrete; before it, it's hypothetical.

---

## Open questions (decide when relevant)

These don't block Phase 2.1; they're parked.

1. **Push commits upstream?** No, not as a PR. Maybe as a "here are
   some bits if anyone wants them" sometime in the future. Decided
   2026-05-15.

2. **Rename the GitHub repo?** Currently `mdc159/OpenFoundry`. If
   this becomes a longer-term project it deserves its own name (e.g.,
   `Lookout`, `Watchtower`). Defer until the project has a real
   identity beyond "evaluation that got out of hand."

3. **Multi-user vs solo?** Currently single-user with one local
   identity. Multi-user requires more backend services running, an
   actual organization concept in `tenancy-organizations-service`,
   etc. Deferred until there's a second user.

4. **Phase 2.6 storage:** Cassandra (already running, but no obvious
   schema for sparse time series) or Iceberg (designed for this, but
   adds query latency). Decide when starting 2.6.

5. **Cesium Ion token:** Currently using the default bundled token,
   which is quota-limited and shared with all Cesium installs. For
   long-term solo eval this is fine; for a public demo we'd want
   our own free Ion account. Defer until quota becomes a problem.

---

## How this doc evolves

When a Phase 2 item starts, copy its card into a working
`docs/superpowers/plans/2026-MM-DD-<feature>.md` file written via the
`writing-plans` skill, expand to TDD step-by-step granularity, then
strike the item here once the plan is in place. After the feature
ships, update this doc with a Phase 1-style retrospective entry.
