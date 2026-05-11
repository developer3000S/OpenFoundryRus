#!/usr/bin/env python3
"""Validate and seed the synthetic Trail Running demo fixture pack."""

from __future__ import annotations

import argparse
import csv
import importlib.util
import json
import shutil
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parent
FIXTURES = ROOT / "fixtures"
WORKSHOP = ROOT / "workshop"
STRAVA_PIPELINE = ROOT / "pipelines" / "strava_activity_ingestion.py"
GPX_PIPELINE = ROOT / "pipelines" / "gpx_trail_ingestion.py"
COFFEE_PIPELINE = ROOT / "pipelines" / "coffee_recommendations.py"
EFFORT_FUNCTION = ROOT / "functions" / "effort_estimator.py"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixtures", type=Path, default=FIXTURES, help="Fixture directory to read.")
    parser.add_argument("--output", type=Path, help="Directory where normalized seed files are written.")
    parser.add_argument("--validate", action="store_true", help="Validate fixtures without requiring an output directory.")
    args = parser.parse_args()

    fixtures = args.fixtures.resolve()
    bundle = build_seed_bundle(fixtures)
    if args.output:
        write_seed_bundle(bundle, args.output.resolve(), fixtures)
        print(f"seeded trail-running demo fixtures into {args.output.resolve()}")
    elif args.validate:
        print("trail-running demo fixtures are valid")
    else:
        parser.error("--output is required unless --validate is set")
    return 0


def build_seed_bundle(fixtures: Path) -> dict[str, Any]:
    manifest = read_json(fixtures / "manifest.json")
    expected = manifest["expected_counts"]
    strava_ingestion = load_transform_module("trail_running_strava_activity_ingestion", STRAVA_PIPELINE)
    gpx_ingestion = load_transform_module("trail_running_gpx_trail_ingestion", GPX_PIPELINE)
    coffee_recommendations = load_transform_module("trail_running_coffee_recommendations", COFFEE_PIPELINE)
    effort_estimator = load_transform_module("trail_running_effort_estimator", EFFORT_FUNCTION)

    activity_doc = read_json(fixtures / manifest["files"]["strava_activities"])
    activities = activity_doc["activities"]
    require(len(activities) == expected["strava_activities"], "unexpected Strava activity count")
    normalized_runs = strava_ingestion.normalize_strava_document(activity_doc)
    require(normalized_runs, "at least one run activity is required")
    require(any(a["type"] == "Trail Run" for a in activities), "fixture needs a Trail Run activity")
    require(any(a["type"] not in {"Run", "Trail Run"} for a in activities), "fixture needs a non-run filter case")

    gpx_source_files = manifest["files"]["gpx_trails"]
    trails = gpx_ingestion.normalize_gpx_files([fixtures / rel for rel in gpx_source_files], gpx_source_files)
    require(len(trails) == expected["gpx_trails"], "unexpected GPX trail count")
    trail_effort_estimates = effort_estimator.estimate_effort(trails, normalized_runs)
    require(len(trail_effort_estimates) == len(trails), "unexpected trail effort estimate count")

    coffee_csv = load_coffee_csv(fixtures / manifest["files"]["coffee_shops_csv"])
    coffee_json = read_json(fixtures / manifest["files"]["coffee_shops_json"])
    require(len(coffee_csv) == expected["coffee_shops"], "unexpected coffee CSV count")
    require(len(coffee_json) == expected["coffee_shops"], "unexpected coffee JSON count")
    validate_coffee_mirror(coffee_csv, coffee_json)
    coffee_bundle = coffee_recommendations.build_coffee_recommendations(trails, coffee_json, nearest_n=3)
    trail_coffee_recommendations = coffee_bundle["trail_coffee_recommendations"]
    trail_coffee_links = coffee_bundle["trail_coffee_links"]
    require(len(trail_coffee_recommendations) == len(trails) * 3, "unexpected coffee recommendation count")
    require(len(trail_coffee_links) == len(trail_coffee_recommendations), "unexpected coffee link count")

    weather = read_json(fixtures / manifest["files"]["weather_response"])
    validate_weather(weather)

    return {
        "manifest": {
            "id": manifest["id"],
            "source": "tools/demo/trail-running",
            "privacy": manifest["privacy"],
            "counts": {
                "strava_activities": len(activities),
                "run_activities": len(normalized_runs),
                "trails": len(trails),
                "trail_effort_estimates": len(trail_effort_estimates),
                "coffee_shops": len(coffee_json),
                "trail_coffee_recommendations": len(trail_coffee_recommendations),
                "trail_coffee_links": len(trail_coffee_links),
                "weather_snapshots": 1,
            },
        },
        "strava_activities": activities,
        "run_activities": normalized_runs,
        "trails": trails,
        "trail_effort_estimates": trail_effort_estimates,
        "coffee_shops": coffee_json,
        "trail_coffee_recommendations": trail_coffee_recommendations,
        "trail_coffee_links": trail_coffee_links,
        "weather_snapshot": normalize_weather(weather),
    }


def write_seed_bundle(bundle: dict[str, Any], output: Path, fixtures: Path) -> None:
    if output.exists():
        if not output.is_dir():
            raise ValueError(f"output exists and is not a directory: {output}")
    output.mkdir(parents=True, exist_ok=True)
    write_json(output / "manifest.json", bundle["manifest"])
    write_json(output / "strava_activities.json", bundle["strava_activities"])
    write_json(output / "run_activities.json", bundle["run_activities"])
    write_json(output / "trails.json", bundle["trails"])
    write_json(output / "trail_effort_estimates.json", bundle["trail_effort_estimates"])
    write_json(output / "coffee_shops.json", bundle["coffee_shops"])
    write_json(output / "trail_coffee_recommendations.json", bundle["trail_coffee_recommendations"])
    write_json(output / "trail_coffee_links.json", bundle["trail_coffee_links"])
    write_json(output / "weather_snapshot.json", bundle["weather_snapshot"])
    shutil.copyfile(fixtures / "coffee_shops.csv", output / "coffee_shops.csv")
    if WORKSHOP.exists():
        shutil.copytree(WORKSHOP, output / "workshop", dirs_exist_ok=True)


def load_transform_module(module_name: str, path: Path) -> Any:
    spec = importlib.util.spec_from_file_location(module_name, path)
    if spec is None or spec.loader is None:
        raise ValueError(f"unable to load demo transform from {path}")
    module = importlib.util.module_from_spec(spec)
    old_dont_write_bytecode = sys.dont_write_bytecode
    sys.dont_write_bytecode = True
    try:
        spec.loader.exec_module(module)
    finally:
        sys.dont_write_bytecode = old_dont_write_bytecode
    return module


def load_coffee_csv(path: Path) -> list[dict[str, Any]]:
    with path.open(newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    for row in rows:
        row["latitude"] = float(row["latitude"])
        row["longitude"] = float(row["longitude"])
        row["rating"] = float(row["rating"])
        row["walkup_window"] = row["walkup_window"].lower() == "true"
        validate_latlon(row["latitude"], row["longitude"], f"coffee shop {row['coffee_shop_id']}")
        require(row["source"] == "synthetic", "coffee shop source must be synthetic")
    return rows


def validate_coffee_mirror(csv_rows: list[dict[str, Any]], json_rows: list[dict[str, Any]]) -> None:
    by_id = {row["coffee_shop_id"]: row for row in csv_rows}
    require(set(by_id) == {row["coffee_shop_id"] for row in json_rows}, "coffee CSV/JSON ids must match")
    for row in json_rows:
        csv_row = by_id[row["coffee_shop_id"]]
        require(row["name"] == csv_row["name"], f"coffee name mismatch for {row['coffee_shop_id']}")
        require(abs(float(row["latitude"]) - csv_row["latitude"]) < 0.000001, "coffee latitude mismatch")
        require(abs(float(row["longitude"]) - csv_row["longitude"]) < 0.000001, "coffee longitude mismatch")
        require(bool(row["walkup_window"]) == csv_row["walkup_window"], "coffee walkup flag mismatch")


def validate_weather(weather: dict[str, Any]) -> None:
    validate_latlon(float(weather["latitude"]), float(weather["longitude"]), "weather response")
    current = weather["current"]
    for key in ["time", "temperature_2m", "relative_humidity_2m", "wind_speed_10m", "wind_direction_10m"]:
        require(key in current, f"weather current response missing {key}")


def normalize_weather(weather: dict[str, Any]) -> dict[str, Any]:
    current = weather["current"]
    return {
        "snapshot_id": "weather-boulder-2026-05-11T14-00",
        "source": weather["source"],
        "latitude": weather["latitude"],
        "longitude": weather["longitude"],
        "time": current["time"],
        "temperature_f": current["temperature_2m"],
        "humidity_percent": current["relative_humidity_2m"],
        "wind_speed_mph": current["wind_speed_10m"],
        "wind_direction_degrees": current["wind_direction_10m"],
    }


def read_json(path: Path) -> Any:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def validate_latlon(lat: float, lon: float, label: str) -> None:
    require(-90.0 <= lat <= 90.0, f"{label} latitude out of range")
    require(-180.0 <= lon <= 180.0, f"{label} longitude out of range")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"trail-running fixture seed failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
