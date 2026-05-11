#!/usr/bin/env python3
"""Parse synthetic GPX trail files into map-ready Trail rows."""

from __future__ import annotations

import argparse
import json
import math
import sys
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Any


METERS_TO_MILES = 0.000621371192237334
METERS_TO_FEET = 3.280839895013123
EARTH_RADIUS_M = 6371008.8
GPX_CONTENT_KEYS = ("gpx", "gpx_xml", "gpx_content", "raw_gpx", "content", "file_content", "body")


def normalize_gpx_files(paths: list[Path], source_files: list[str] | None = None) -> list[dict[str, Any]]:
    rows = []
    for index, path in enumerate(paths):
        source_file = source_files[index] if source_files and index < len(source_files) else source_file_for_cli(path)
        rows.append(parse_gpx_file(path, source_file))
    if not rows:
        raise ValueError("at least one GPX file is required")
    return rows


def parse_gpx_file(path: Path, source_file: str | None = None) -> dict[str, Any]:
    return parse_gpx_document(path.read_text(encoding="utf-8"), source_file or source_file_for_cli(path))


def parse_gpx_document(document: str | bytes, source_file: str) -> dict[str, Any]:
    root = ET.fromstring(document)
    names = [text(root.find("./{*}metadata/{*}name")), text(root.find(".//{*}trk/{*}name")), text(root.find(".//{*}rte/{*}name"))]
    fallback_name = Path(source_file).stem.replace("_", " ").title()
    name = next((value for value in names if value), fallback_name)
    points = []
    for point in list(root.findall(".//{*}trkpt")) + list(root.findall(".//{*}rtept")):
        lat = float(point.attrib["lat"])
        lon = float(point.attrib["lon"])
        validate_latlon(lat, lon, f"{source_file} GPX point")
        ele_value = text(point.find("{*}ele"))
        points.append({"lat": lat, "lon": lon, "elevation_meters": float(ele_value) if ele_value else None})
    require(len(points) >= 2, f"{source_file} must contain at least two points")

    distance_m = 0.0
    gain_m = 0.0
    elevations = [point["elevation_meters"] for point in points if point["elevation_meters"] is not None]
    for previous, current in zip(points, points[1:]):
        distance_m += haversine_m(previous["lat"], previous["lon"], current["lat"], current["lon"])
        if previous["elevation_meters"] is not None and current["elevation_meters"] is not None:
            gain_m += max(0.0, current["elevation_meters"] - previous["elevation_meters"])

    lats = [point["lat"] for point in points]
    lons = [point["lon"] for point in points]
    coords = [[point["lon"], point["lat"]] for point in points]
    bbox = [min(lons), min(lats), max(lons), max(lats)]
    start = points[0]
    end = points[-1]
    min_ele = min(elevations) if elevations else None
    max_ele = max(elevations) if elevations else None
    return {
        "trail_id": Path(source_file).stem.replace("_", "-"),
        "trail_name": name,
        "source_file": source_file,
        "point_count": len(points),
        "distance_meters": round(distance_m, 3),
        "distance_miles": round(distance_m * METERS_TO_MILES, 4),
        "elevation_gain_meters": round(gain_m, 3),
        "elevation_gain_ft": round(gain_m * METERS_TO_FEET, 2),
        "min_elevation_meters": min_ele,
        "max_elevation_meters": max_ele,
        "min_elevation_ft": round(min_ele * METERS_TO_FEET, 2) if min_ele is not None else None,
        "max_elevation_ft": round(max_ele * METERS_TO_FEET, 2) if max_ele is not None else None,
        "start_lat": start["lat"],
        "start_lon": start["lon"],
        "end_lat": end["lat"],
        "end_lon": end["lon"],
        "trailhead_geopoint": f"{start['lat']},{start['lon']}",
        "route_bbox": bbox,
        "route_geojson": {
            "type": "LineString",
            "coordinates": coords,
            "bbox": bbox,
        },
    }


def transform(input_rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    rows = []
    for input_row in input_rows:
        source_file = optional_string(input_row, "source_file", "file_name", "upload_name", "path", "fixture_path") or "inline.gpx"
        content = first_content(input_row)
        if content is not None:
            rows.append(parse_gpx_document(content, source_file))
            continue
        path_value = optional_string(input_row, "fixture_path", "path")
        if path_value:
            rows.append(parse_gpx_file(Path(path_value), source_file_for_cli(Path(path_value))))
            continue
        raise ValueError(f"input row for {source_file} has no GPX content or path")
    if not rows:
        raise ValueError("at least one GPX input row is required")
    return rows


def first_content(row: dict[str, Any]) -> str | None:
    for key in GPX_CONTENT_KEYS:
        value = row.get(key)
        if value is None:
            continue
        if isinstance(value, bytes):
            return value.decode("utf-8")
        if isinstance(value, str):
            return value
        raise ValueError(f"GPX content field {key} must be a string")
    return None


def optional_string(row: dict[str, Any], *keys: str) -> str | None:
    for key in keys:
        value = row.get(key)
        if value is None:
            continue
        if not isinstance(value, str):
            raise ValueError(f"input field {key} must be a string")
        if value.strip():
            return value
    return None


def source_file_for_cli(path: Path) -> str:
    raw = path.as_posix()
    parts = raw.split("/")
    if "fixtures" in parts:
        index = parts.index("fixtures")
        return "/".join(parts[index + 1 :])
    return raw


def validate_latlon(lat: float, lon: float, label: str) -> None:
    require(-90.0 <= lat <= 90.0, f"{label} latitude out of range")
    require(-180.0 <= lon <= 180.0, f"{label} longitude out of range")


def haversine_m(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    phi1 = math.radians(lat1)
    phi2 = math.radians(lat2)
    delta_phi = math.radians(lat2 - lat1)
    delta_lambda = math.radians(lon2 - lon1)
    a = math.sin(delta_phi / 2.0) ** 2 + math.cos(phi1) * math.cos(phi2) * math.sin(delta_lambda / 2.0) ** 2
    return 2.0 * EARTH_RADIUS_M * math.atan2(math.sqrt(a), math.sqrt(1.0 - a))


def text(element: Any) -> str:
    if element is None or element.text is None:
        return ""
    return element.text.strip()


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("inputs", nargs="+", type=Path, help="GPX files to parse.")
    parser.add_argument("--output", type=Path, help="Optional output JSON path.")
    args = parser.parse_args()
    rows = normalize_gpx_files(args.inputs)
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
        print(f"GPX trail ingestion failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
