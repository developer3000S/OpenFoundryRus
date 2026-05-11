#!/usr/bin/env python3
"""Compute nearest coffee shops for each synthetic Trail row."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys
from pathlib import Path
from typing import Any


EARTH_RADIUS_M = 6371008.8
METERS_TO_MILES = 0.000621371192237334
MODEL_VERSION = "haversine_trailhead_to_cafe_v1"
LINK_TYPE_API_NAME = "TrailNearbyCoffeeShop"


def build_coffee_recommendations(
    trails: list[dict[str, Any]],
    coffee_shops: list[dict[str, Any]],
    nearest_n: int = 3,
) -> dict[str, list[dict[str, Any]]]:
    if nearest_n <= 0:
        raise ValueError("nearest_n must be positive")
    if not trails:
        raise ValueError("at least one trail row is required")
    if not coffee_shops:
        raise ValueError("at least one coffee shop row is required")

    cafes = [normalize_coffee_shop(row) for row in coffee_shops]
    recommendations: list[dict[str, Any]] = []
    links: list[dict[str, Any]] = []
    for trail in trails:
        trail_id = required_string(trail, "trail_id")
        trail_name = required_string(trail, "trail_name")
        trail_lat = required_float(trail, "start_lat")
        trail_lon = required_float(trail, "start_lon")
        validate_latlon(trail_lat, trail_lon, f"trail {trail_id}")

        candidates = []
        for cafe in cafes:
            distance_m = haversine_m(trail_lat, trail_lon, cafe["latitude"], cafe["longitude"])
            candidates.append((distance_m, cafe["coffee_shop_id"], cafe))
        candidates.sort(key=lambda item: (item[0], item[1]))

        for rank, (distance_m, _cafe_id, cafe) in enumerate(candidates[:nearest_n], start=1):
            recommendation = build_recommendation_row(
                trail_id=trail_id,
                trail_name=trail_name,
                trail_lat=trail_lat,
                trail_lon=trail_lon,
                cafe=cafe,
                distance_m=distance_m,
                rank=rank,
            )
            recommendations.append(recommendation)
            links.append(build_link_row(recommendation))

    return {
        "trail_coffee_recommendations": recommendations,
        "trail_coffee_links": links,
    }


def transform(input_rows: list[dict[str, Any]], nearest_n: int = 3) -> list[dict[str, Any]]:
    trails = []
    cafes = []
    for row in input_rows:
        if "trail_id" in row and "start_lat" in row and "start_lon" in row:
            trails.append(row)
        elif "coffee_shop_id" in row and "latitude" in row and "longitude" in row:
            cafes.append(row)
    return build_coffee_recommendations(trails, cafes, nearest_n)["trail_coffee_recommendations"]


def build_recommendation_row(
    trail_id: str,
    trail_name: str,
    trail_lat: float,
    trail_lon: float,
    cafe: dict[str, Any],
    distance_m: float,
    rank: int,
) -> dict[str, Any]:
    cafe_id = cafe["coffee_shop_id"]
    distance_miles = distance_m * METERS_TO_MILES
    line_geojson = line_between(trail_lat, trail_lon, cafe["latitude"], cafe["longitude"])
    return {
        "recommendation_id": f"{trail_id}-{rank}-{cafe_id}",
        "trail_id": trail_id,
        "trail_name": trail_name,
        "coffee_shop_id": cafe_id,
        "coffee_shop_name": cafe["name"],
        "rank": rank,
        "distance_meters": round(distance_m, 3),
        "distance_km": round(distance_m / 1000.0, 4),
        "distance_miles": round(distance_miles, 4),
        "trailhead_lat": trail_lat,
        "trailhead_lon": trail_lon,
        "trailhead_geopoint": f"{trail_lat},{trail_lon}",
        "coffee_lat": cafe["latitude"],
        "coffee_lon": cafe["longitude"],
        "coffee_geopoint": f"{cafe['latitude']},{cafe['longitude']}",
        "coffee_neighborhood": cafe["neighborhood"],
        "coffee_rating": cafe["rating"],
        "coffee_walkup_window": cafe["walkup_window"],
        "link_type_api_name": LINK_TYPE_API_NAME,
        "line_geojson": line_geojson,
        "recommendation_model": MODEL_VERSION,
    }


def build_link_row(recommendation: dict[str, Any]) -> dict[str, Any]:
    trail_id = recommendation["trail_id"]
    cafe_id = recommendation["coffee_shop_id"]
    return {
        "link_id": f"trail-coffee-{trail_id}-{cafe_id}",
        "link_type_api_name": LINK_TYPE_API_NAME,
        "source_object_type": "Trail",
        "source_object_id": trail_id,
        "target_object_type": "CoffeeShop",
        "target_object_id": cafe_id,
        "trail_id": trail_id,
        "coffee_shop_id": cafe_id,
        "rank": recommendation["rank"],
        "distance_meters": recommendation["distance_meters"],
        "distance_miles": recommendation["distance_miles"],
        "line_geojson": recommendation["line_geojson"],
        "relationship_label": "nearest coffee shop",
        "recommendation_model": MODEL_VERSION,
    }


def normalize_coffee_shop(row: dict[str, Any]) -> dict[str, Any]:
    cafe_id = required_string(row, "coffee_shop_id")
    lat = required_float(row, "latitude")
    lon = required_float(row, "longitude")
    validate_latlon(lat, lon, f"coffee shop {cafe_id}")
    return {
        "coffee_shop_id": cafe_id,
        "name": required_string(row, "name"),
        "latitude": lat,
        "longitude": lon,
        "neighborhood": required_string(row, "neighborhood"),
        "rating": required_float(row, "rating"),
        "walkup_window": required_bool(row, "walkup_window"),
    }


def line_between(lat1: float, lon1: float, lat2: float, lon2: float) -> dict[str, Any]:
    min_lon = min(lon1, lon2)
    min_lat = min(lat1, lat2)
    max_lon = max(lon1, lon2)
    max_lat = max(lat1, lat2)
    return {
        "type": "LineString",
        "coordinates": [[lon1, lat1], [lon2, lat2]],
        "bbox": [min_lon, min_lat, max_lon, max_lat],
    }


def haversine_m(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    phi1 = math.radians(lat1)
    phi2 = math.radians(lat2)
    delta_phi = math.radians(lat2 - lat1)
    delta_lambda = math.radians(lon2 - lon1)
    a = math.sin(delta_phi / 2.0) ** 2 + math.cos(phi1) * math.cos(phi2) * math.sin(delta_lambda / 2.0) ** 2
    return 2.0 * EARTH_RADIUS_M * math.atan2(math.sqrt(a), math.sqrt(1.0 - a))


def validate_latlon(lat: float, lon: float, label: str) -> None:
    if not -90.0 <= lat <= 90.0:
        raise ValueError(f"{label} latitude out of range")
    if not -180.0 <= lon <= 180.0:
        raise ValueError(f"{label} longitude out of range")


def required_string(row: dict[str, Any], key: str) -> str:
    value = row.get(key)
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"field {key} must be a non-empty string")
    return value


def required_float(row: dict[str, Any], key: str) -> float:
    value = row.get(key)
    if isinstance(value, bool) or value is None:
        raise ValueError(f"field {key} must be numeric")
    try:
        return float(value)
    except (TypeError, ValueError) as exc:
        raise ValueError(f"field {key} must be numeric") from exc


def required_bool(row: dict[str, Any], key: str) -> bool:
    value = row.get(key)
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        lowered = value.strip().lower()
        if lowered in {"true", "1", "yes"}:
            return True
        if lowered in {"false", "0", "no"}:
            return False
    raise ValueError(f"field {key} must be boolean")


def read_rows(path: Path) -> list[dict[str, Any]]:
    if path.suffix.lower() == ".csv":
        with path.open(newline="", encoding="utf-8") as handle:
            return list(csv.DictReader(handle))
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, list):
        raise ValueError(f"{path} must contain a JSON list")
    return payload


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--trails", required=True, type=Path, help="Trail rows JSON.")
    parser.add_argument("--coffee-shops", required=True, type=Path, help="Coffee shop rows JSON or CSV.")
    parser.add_argument("--nearest-n", type=int, default=3, help="Nearest coffee shops per trail.")
    parser.add_argument("--output", type=Path, help="Optional bundle output JSON path.")
    parser.add_argument("--recommendations-output", type=Path, help="Optional recommendations output JSON path.")
    parser.add_argument("--links-output", type=Path, help="Optional link-table output JSON path.")
    args = parser.parse_args()

    bundle = build_coffee_recommendations(read_rows(args.trails), read_rows(args.coffee_shops), args.nearest_n)
    if args.output:
        write_json(args.output, bundle)
    if args.recommendations_output:
        write_json(args.recommendations_output, bundle["trail_coffee_recommendations"])
    if args.links_output:
        write_json(args.links_output, bundle["trail_coffee_links"])
    if not args.output and not args.recommendations_output and not args.links_output:
        print(json.dumps(bundle, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"coffee recommendation pipeline failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
