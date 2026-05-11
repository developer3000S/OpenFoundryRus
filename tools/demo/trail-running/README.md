# Trail Running Demo Fixture Pack

Synthetic fixture pack for the Foundry-style Trail Running demo acceptance
scenario. The data is intentionally small and deterministic so Pipeline Builder,
Ontology outputs, Data Connection webhooks, and Workshop widgets can be tested
without private Strava exports or personal location history.

## Files

- `fixtures/strava_activities.json`: Strava-like run and trail-run activity
  export with normalized-friendly metrics.
- `fixtures/gpx/*.gpx`: three synthetic Boulder-area GPX trail files plus
  `custom_dawn_ridge.gpx` for Workshop upload acceptance.
- `fixtures/coffee_shops.csv` and `fixtures/coffee_shops.json`: matching
  coffee-shop tables with GeoPoint-ready latitude/longitude columns.
- `fixtures/weather_open_meteo_boulder.json`: saved Open-Meteo-style response
  for offline weather-card and webhook/action tests.
- `fixtures/manifest.json`: fixture inventory and expected row counts.
- `data_connections/open_meteo_weather.source.json`: REST API source and
  webhook contract for Open-Meteo current weather.
- `actions/fetch_trail_weather.action.json`: generic Ontology action contract
  for fetching weather and writing `WeatherSnapshot` rows.
- `actions/fetch_trail_weather.py`: local acceptance runner for the
  webhook/action contract with mock HTTP support.
- `ontology/weather_snapshot.object_type.json`: object type metadata for the
  weather writeback target.
- `workshop/run_fast.workshop_app.json`: published Workshop app contract with
  overview, map, selected-trail detail, and GPX upload pages for runtime
  acceptance tests.
- `workshop/gpx_upload_flow.py`: local acceptance runner for the Workshop GPX
  upload path, using the GPX parser and effort estimator.
- `pipelines/strava_activity_ingestion.py`: Python transform that creates
  normalized `RunActivity` rows from the synthetic Strava-like export.
- `pipelines/strava_activity_ingestion.pipeline.json`: Pipeline IR contract for
  the Strava ingestion graph, including dataset and Ontology object outputs.
- `pipelines/gpx_trail_ingestion.py`: Python transform that parses GPX trails
  into map-ready `Trail` rows.
- `pipelines/gpx_trail_ingestion.pipeline.json`: Pipeline IR contract for the
  GPX ingestion graph, including `gpx_parse`, dataset output, and Ontology
  object output nodes.
- `pipelines/coffee_recommendations.py`: Haversine-based transform that ranks
  nearest coffee shops for each trailhead.
- `pipelines/coffee_recommendations.pipeline.json`: Pipeline IR contract for
  the coffee recommendation graph, including nearest-neighbor geospatial join,
  CoffeeShop outputs, recommendation outputs, and a link-table dataset.
- `functions/effort_estimator.py`: function-style estimator that scores Trail
  rows against historical `RunActivity` rows.
- `functions/effort_estimator.function.json`: function contract for Workshop
  function-backed variables.
- `expected/run_activities.golden.json`: deterministic `RunActivity` output
  used by tests and downstream demo steps.
- `expected/trails.golden.json`: deterministic `Trail` output with GeoPoint,
  bbox, and GeoJSON LineString fields.
- `expected/trail_effort_estimates.golden.json`: deterministic
  `TrailEffortEstimate` output with top similar runs and averaged metrics.
- `expected/trail_coffee_recommendations.golden.json`: deterministic nearest
  coffee-shop table with Haversine distances and map line geometry.
- `expected/trail_coffee_links.golden.json`: deterministic Trail-to-CoffeeShop
  link table for object traversal and map overlays.
- `expected/trail_weather_snapshot.golden.json`: deterministic
  `WeatherSnapshot` writeback object for the mocked Open-Meteo action.
- `expected/custom_gpx_upload_trail.golden.json` and
  `expected/custom_gpx_upload_estimate.golden.json`: deterministic objects
  produced when the Workshop upload widget parses `custom_dawn_ridge.gpx`.

## Seed Locally

```bash
python3 tools/demo/trail-running/seed.py --output /tmp/openfoundry-trail-demo
```

The seed command validates every fixture and writes normalized demo inputs:

- `run_activities.json`
- `trails.json`
- `trail_effort_estimates.json`
- `coffee_shops.csv`
- `coffee_shops.json`
- `trail_coffee_recommendations.json`
- `trail_coffee_links.json`
- `weather_snapshot.json`
- `workshop/run_fast.workshop_app.json`
- `manifest.json`

Use `--validate` when a CI or docs job only needs to prove the committed pack is
healthy.

## Strava Ingestion Pipeline

```bash
python3 tools/demo/trail-running/pipelines/strava_activity_ingestion.py \
  tools/demo/trail-running/fixtures/strava_activities.json \
  --output /tmp/openfoundry-trail-demo/run_activities.json
```

The transform filters out non-running activities, rejects private athlete or
device-style keys, normalizes meters to miles and feet, derives pace in minutes
per mile, carries heart-rate and perceived-effort metrics, and emits the
`RunActivity` schema used by the Pipeline Builder and Ontology output fixtures.
The synthetic rows also carry terrain, surface, altitude, and weather profile
fields used by the smarter similarity model.

## GPX Trail Ingestion Pipeline

```bash
python3 tools/demo/trail-running/pipelines/gpx_trail_ingestion.py \
  tools/demo/trail-running/fixtures/gpx/mesa_overlook_loop.gpx \
  tools/demo/trail-running/fixtures/gpx/green_mountain_ascent.gpx \
  tools/demo/trail-running/fixtures/gpx/boulder_creek_path.gpx \
  --output /tmp/openfoundry-trail-demo/trails.json
```

The transform validates WGS84 lat/lon coordinates, computes distance and
elevation gain, records start/end points, emits an Ontology-style trailhead
GeoPoint string, and builds a GeoJSON LineString plus bbox for MapLibre-backed
Workshop map widgets.

## Effort Estimator Function

```bash
python3 tools/demo/trail-running/functions/effort_estimator.py \
  --trails tools/demo/trail-running/expected/trails.golden.json \
  --runs tools/demo/trail-running/expected/run_activities.golden.json \
  --top-n 5 \
  --output /tmp/openfoundry-trail-demo/trail_effort_estimates.json
```

The function builds a weighted KNN-style profile over distance, elevation gain,
gain per mile, altitude, terrain, surface, weather load, and HR profile. It
ranks historical runs by weighted vector distance, averages pace, effort, and
heart-rate metrics over the top matches, and emits confidence values plus
feature-level explanations for Workshop metric cards and selected-trail detail
panels.

## Coffee Recommendation Pipeline

```bash
python3 tools/demo/trail-running/pipelines/coffee_recommendations.py \
  --trails tools/demo/trail-running/expected/trails.golden.json \
  --coffee-shops tools/demo/trail-running/fixtures/coffee_shops.json \
  --nearest-n 3 \
  --recommendations-output /tmp/openfoundry-trail-demo/trail_coffee_recommendations.json \
  --links-output /tmp/openfoundry-trail-demo/trail_coffee_links.json
```

The transform validates WGS84 trailhead and cafe coordinates, computes
Haversine distances in meters, kilometers, and miles, ranks the nearest cafes
per trail, emits a line GeoJSON between each trailhead and cafe, and writes a
link-table shaped output for Workshop map/table flows.

## Weather Webhook Action

```bash
python3 tools/demo/trail-running/actions/fetch_trail_weather.py \
  --source tools/demo/trail-running/data_connections/open_meteo_weather.source.json \
  --action tools/demo/trail-running/actions/fetch_trail_weather.action.json \
  --trails tools/demo/trail-running/expected/trails.golden.json \
  --trail-id boulder-creek-path \
  --mock-response tools/demo/trail-running/fixtures/weather_open_meteo_boulder.json \
  --output /tmp/openfoundry-trail-demo/trail_weather_snapshot.json
```

The source contract models an invokable Open-Meteo REST webhook with lat/lon
inputs, typed extractors, rate/concurrency limits, and sanitized history
settings. The action contract maps the selected Trail into webhook inputs,
stores parsed outputs under `webhook_output`, and writes a `WeatherSnapshot`
object payload that Workshop Button Group can trigger for the active trail.

## Custom GPX Upload From Workshop

```bash
python3 tools/demo/trail-running/workshop/gpx_upload_flow.py \
  --gpx tools/demo/trail-running/fixtures/gpx/custom_dawn_ridge.gpx \
  --runs tools/demo/trail-running/expected/run_activities.golden.json \
  --trail-output /tmp/openfoundry-trail-demo/custom_gpx_upload_trail.json \
  --estimate-output /tmp/openfoundry-trail-demo/custom_gpx_upload_estimate.json
```

The Workshop upload path sends the GPX file to
`/api/v1/pipelines/geospatial/gpx/parse`, normalizes the parser output into a
`Trail` object, invokes `estimateTrailEffort`, creates a
`TrailEffortEstimate` object, and refreshes object-set backed widgets. The
browser smoke is:

```bash
pnpm --dir apps/web test:e2e -- workshop-gpx-upload.spec.ts
```

## Workshop App Runtime

```bash
pnpm --dir apps/web test:e2e -- workshop-trail-running-demo.spec.ts
```

The Workshop app fixture publishes `run-fast` with four pages:

- Trail Overview: Filter List, Object Table, Chart XY, Metric Card, and Weather
  Button Group widgets.
- Trail Map: MapLibre-backed trailhead, route, coffee-shop, and
  trail-to-coffee distance layers plus a nearest coffee Object Table.
- Trail Detail: Object Set Title, Property List, Metric Card, and coffee
  recommendation table for the default selected trail.
- Upload GPX: custom GPX upload widget plus Trail and TrailEffortEstimate
  tables that update after the upload flow creates objects.

The Playwright smokes open `/apps/runtime/run-fast`, navigate the published
runtime pages, exercise the custom GPX upload flow, and serve object-set data
from the deterministic fixture outputs in this directory.
