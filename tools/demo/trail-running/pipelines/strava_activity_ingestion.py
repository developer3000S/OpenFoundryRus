#!/usr/bin/env python3
"""Normalize a synthetic Strava activity export into RunActivity rows."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


METERS_TO_MILES = 0.000621371192237334
METERS_TO_FEET = 3.280839895013123
RUN_TYPES = {"Run", "Trail Run"}
PRIVATE_KEYS = {
    "athlete",
    "athlete_id",
    "device_id",
    "email",
    "firstname",
    "lastname",
    "profile",
    "profile_medium",
    "resource_state",
}


def normalize_strava_document(document: dict[str, Any] | list[dict[str, Any]]) -> list[dict[str, Any]]:
    reject_private_keys(document, "strava activity export")
    if isinstance(document, list):
        activities = document
    else:
        activities = document.get("activities", [])
    if not isinstance(activities, list):
        raise ValueError("strava activity export must contain an activities array")
    rows = [normalize_activity(activity) for activity in activities if activity.get("type") in RUN_TYPES]
    if not rows:
        raise ValueError("strava activity export must contain at least one Run or Trail Run")
    return rows


def normalize_activity(activity: dict[str, Any]) -> dict[str, Any]:
    activity_id = required_string(activity, "activity_id")
    distance_meters = required_float(activity, "distance_meters")
    moving_time_seconds = required_int(activity, "moving_time_seconds")
    elevation_gain_meters = required_float(activity, "total_elevation_gain_meters")
    if distance_meters <= 0:
        raise ValueError(f"activity {activity_id} distance_meters must be positive")
    if moving_time_seconds <= 0:
        raise ValueError(f"activity {activity_id} moving_time_seconds must be positive")
    distance_miles = distance_meters * METERS_TO_MILES
    pace_min_per_mile = moving_time_seconds / 60.0 / distance_miles
    start_lat, start_lon = latlon_pair(activity.get("start_latlng"), f"{activity_id} start_latlng")
    end_lat, end_lon = latlon_pair(activity.get("end_latlng", activity.get("start_latlng")), f"{activity_id} end_latlng")
    return {
        "activity_id": activity_id,
        "activity_name": required_string(activity, "name"),
        "activity_type": required_string(activity, "type"),
        "start_date_local": required_string(activity, "start_date_local"),
        "distance_meters": round(distance_meters, 3),
        "distance_miles": round(distance_miles, 4),
        "moving_time_seconds": moving_time_seconds,
        "moving_time_minutes": round(moving_time_seconds / 60.0, 2),
        "pace_min_per_mile": round(pace_min_per_mile, 2),
        "elevation_gain_meters": round(elevation_gain_meters, 3),
        "elevation_gain_ft": round(elevation_gain_meters * METERS_TO_FEET, 2),
        "average_heartrate": optional_float(activity, "average_heartrate"),
        "max_heartrate": optional_float(activity, "max_heartrate"),
        "perceived_effort": optional_int(activity, "perceived_effort"),
        "terrain": optional_string(activity, "terrain"),
        "surface": optional_string(activity, "surface"),
        "average_elevation_ft": optional_float(activity, "average_elevation_ft"),
        "temperature_f": optional_float(activity, "temperature_f"),
        "humidity_percent": optional_float(activity, "humidity_percent"),
        "wind_speed_mph": optional_float(activity, "wind_speed_mph"),
        "start_lat": start_lat,
        "start_lon": start_lon,
        "end_lat": end_lat,
        "end_lon": end_lon,
        "source": "synthetic-strava-export",
    }


def transform(input_rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for row in input_rows:
        payload = row.get("payload", row.get("strava_activities_json", row))
        if isinstance(payload, str):
            payload = json.loads(payload)
        rows.extend(normalize_strava_document(payload))
    return rows


def reject_private_keys(value: Any, location: str) -> None:
    if isinstance(value, dict):
        for key, nested in value.items():
            if key.lower() in PRIVATE_KEYS:
                raise ValueError(f"{location} contains private key {key!r}")
            reject_private_keys(nested, location)
    elif isinstance(value, list):
        for nested in value:
            reject_private_keys(nested, location)


def required_string(activity: dict[str, Any], key: str) -> str:
    value = activity.get(key)
    if not isinstance(value, str) or value.strip() == "":
        raise ValueError(f"activity is missing required string {key}")
    return value


def required_float(activity: dict[str, Any], key: str) -> float:
    if key not in activity:
        raise ValueError(f"activity is missing required number {key}")
    return float(activity[key])


def required_int(activity: dict[str, Any], key: str) -> int:
    if key not in activity:
        raise ValueError(f"activity is missing required integer {key}")
    return int(activity[key])


def optional_float(activity: dict[str, Any], key: str) -> float | None:
    if activity.get(key) is None:
        return None
    return float(activity[key])


def optional_int(activity: dict[str, Any], key: str) -> int | None:
    if activity.get(key) is None:
        return None
    return int(activity[key])


def optional_string(activity: dict[str, Any], key: str) -> str | None:
    value = activity.get(key)
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError(f"activity field {key} must be a string")
    return value


def latlon_pair(value: Any, label: str) -> tuple[float, float]:
    if not isinstance(value, list) or len(value) != 2:
        raise ValueError(f"{label} must be [lat, lon]")
    lat = float(value[0])
    lon = float(value[1])
    if lat < -90 or lat > 90:
        raise ValueError(f"{label} latitude out of range")
    if lon < -180 or lon > 180:
        raise ValueError(f"{label} longitude out of range")
    return lat, lon


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path, help="Strava-like JSON export.")
    parser.add_argument("--output", type=Path, help="Optional output JSON path.")
    args = parser.parse_args()
    with args.input.open(encoding="utf-8") as handle:
        document = json.load(handle)
    rows = normalize_strava_document(document)
    body = json.dumps(rows, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(body, encoding="utf-8")
    else:
        print(body, end="")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"strava activity ingestion failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
