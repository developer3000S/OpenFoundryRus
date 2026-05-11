#!/usr/bin/env python3
"""Estimate trail running effort from similar historical RunActivity rows."""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Any


DEFAULT_TOP_N = 5
MODEL_VERSION = "weighted_knn_trail_profile_v2"
DEFAULT_WEATHER_PROFILE = {
    "temperature_f": 68.4,
    "humidity_percent": 34.0,
    "wind_speed_mph": 6.2,
}
FEATURE_WEIGHTS = {
    "distance": 0.24,
    "elevation_gain": 0.18,
    "grade": 0.14,
    "altitude": 0.10,
    "terrain": 0.10,
    "surface": 0.06,
    "weather": 0.08,
    "hr_profile": 0.10,
}
TERRAIN_SCORES = {
    "flat": 0.0,
    "road": 0.05,
    "path": 0.20,
    "rolling": 0.45,
    "hilly": 0.70,
    "mountain": 1.0,
}
SURFACE_SCORES = {
    "road": 0.0,
    "mixed": 0.45,
    "trail": 1.0,
}


def estimate_effort(
    trails: list[dict[str, Any]],
    run_activities: list[dict[str, Any]],
    top_n: int = DEFAULT_TOP_N,
    weather_profile: dict[str, Any] | None = None,
) -> list[dict[str, Any]]:
    if top_n <= 0:
        raise ValueError("top_n must be positive")
    if not trails:
        raise ValueError("at least one Trail row is required")
    runs = [run for run in run_activities if run.get("activity_type") in {"Run", "Trail Run"}]
    if not runs:
        raise ValueError("at least one RunActivity row is required")

    weather = normalize_weather_profile(weather_profile)
    trail_profiles = {required_string(trail, "trail_id"): trail_profile(trail, weather) for trail in trails}
    run_profiles = {required_string(run, "activity_id"): run_profile(run) for run in runs}
    scales = profile_scales([*trail_profiles.values(), *run_profiles.values()])
    estimates = []
    for trail in sorted(trails, key=lambda row: required_string(row, "trail_id")):
        profile = trail_profiles[required_string(trail, "trail_id")]
        matches = rank_similar_runs(trail, profile, runs, run_profiles, scales)[:top_n]
        estimates.append(build_estimate(trail, profile, matches, scales))
    return estimates


def rank_similar_runs(
    trail: dict[str, Any],
    profile: dict[str, Any],
    run_activities: list[dict[str, Any]],
    run_profiles: dict[str, dict[str, Any]],
    scales: dict[str, float],
) -> list[dict[str, Any]]:
    _ = trail
    ranked = []
    for run in run_activities:
        activity_id = required_string(run, "activity_id")
        candidate = run_profiles[activity_id]
        component_scores = similarity_components(profile, candidate, scales)
        score = weighted_score(component_scores)
        vector_delta = similarity_vector_delta(profile, candidate, scales)
        match_basis = explain_match(component_scores, profile, candidate)
        ranked.append(
            {
                "activity_id": activity_id,
                "activity_name": required_string(run, "activity_name"),
                "activity_type": required_string(run, "activity_type"),
                "distance_miles": round(candidate["distance_miles"], 4),
                "elevation_gain_ft": round(candidate["elevation_gain_ft"], 2),
                "gain_per_mile": round(candidate["gain_per_mile"], 2),
                "average_elevation_ft": round(candidate["average_elevation_ft"], 2),
                "terrain": candidate["terrain"],
                "surface": candidate["surface"],
                "temperature_f": round(candidate["temperature_f"], 1),
                "humidity_percent": round(candidate["humidity_percent"], 1),
                "wind_speed_mph": round(candidate["wind_speed_mph"], 1),
                "pace_min_per_mile": required_float(run, "pace_min_per_mile"),
                "perceived_effort": optional_float(run, "perceived_effort"),
                "average_heartrate": optional_float(run, "average_heartrate"),
                "max_heartrate": optional_float(run, "max_heartrate"),
                "similarity_score": round(score, 4),
                "weighted_similarity_score": round(score, 4),
                "normalized_distance_delta": round(vector_delta["distance"], 4),
                "normalized_elevation_delta": round(vector_delta["elevation_gain"], 4),
                "normalized_grade_delta": round(vector_delta["grade"], 4),
                "normalized_altitude_delta": round(vector_delta["altitude"], 4),
                "terrain_delta": round(vector_delta["terrain"], 4),
                "surface_delta": round(vector_delta["surface"], 4),
                "weather_delta": round(vector_delta["weather"], 4),
                "hr_profile_delta": round(vector_delta["hr_profile"], 4),
                "feature_scores": round_dict(component_scores, 4),
                "weighted_feature_scores": round_dict(weighted_feature_scores(component_scores), 4),
                "similarity_vector_delta": round_dict(vector_delta, 4),
                "match_explanation": match_basis,
            }
        )
    return sorted(ranked, key=lambda row: (row["similarity_score"], row["activity_id"]))


def build_estimate(
    trail: dict[str, Any],
    profile: dict[str, Any],
    matches: list[dict[str, Any]],
    scales: dict[str, float],
) -> dict[str, Any]:
    if not matches:
        raise ValueError(f"trail {required_string(trail, 'trail_id')} has no similar runs")
    avg_similarity = average([required_float(match, "similarity_score") for match in matches])
    confidence = max(0.0, min(1.0, 1.0 / (1.0 + avg_similarity)))
    return {
        "estimate_id": f"effort-{required_string(trail, 'trail_id')}",
        "trail_id": required_string(trail, "trail_id"),
        "trail_name": required_string(trail, "trail_name"),
        "distance_miles": round(required_float(trail, "distance_miles"), 4),
        "elevation_gain_ft": round(required_float(trail, "elevation_gain_ft"), 2),
        "similar_run_count": len(matches),
        "similar_run_ids": [required_string(match, "activity_id") for match in matches],
        "similar_runs": matches,
        "estimated_pace_min_per_mile": round(average([required_float(match, "pace_min_per_mile") for match in matches]), 2),
        "estimated_perceived_effort": round(average_optional([match.get("perceived_effort") for match in matches]), 2),
        "estimated_average_heartrate": round(average_optional([match.get("average_heartrate") for match in matches]), 1),
        "estimated_max_heartrate": round(average_optional([match.get("max_heartrate") for match in matches]), 1),
        "best_similarity_score": matches[0]["similarity_score"],
        "average_similarity_score": round(avg_similarity, 4),
        "confidence": round(confidence, 3),
        "similarity_model": MODEL_VERSION,
        "feature_weights": round_dict(FEATURE_WEIGHTS, 4),
        "target_profile": public_profile(profile),
        "target_similarity_vector": round_dict(similarity_vector(profile, scales), 4),
        "similarity_explanation": {
            "method": "weighted KNN over trail profile features",
            "score_direction": "lower is more similar",
            "features": list(FEATURE_WEIGHTS.keys()),
            "top_match_basis": matches[0]["match_explanation"],
        },
        "normalization": round_dict(scales, 4),
    }


def trail_profile(trail: dict[str, Any], weather: dict[str, float]) -> dict[str, Any]:
    distance = required_float(trail, "distance_miles")
    gain = required_float(trail, "elevation_gain_ft")
    min_elevation = optional_float(trail, "min_elevation_ft")
    max_elevation = optional_float(trail, "max_elevation_ft")
    altitude = optional_float(trail, "average_elevation_ft")
    if altitude is None and min_elevation is not None and max_elevation is not None:
        altitude = (min_elevation + max_elevation) / 2.0
    if altitude is None:
        altitude = 5280.0 + gain * 0.2
    grade = gain_per_mile(gain, distance)
    terrain = normalize_label(optional_string(trail, "terrain")) or infer_terrain(grade)
    surface = normalize_label(optional_string(trail, "surface")) or infer_surface(terrain, grade)
    temperature = optional_float(trail, "temperature_f")
    humidity = optional_float(trail, "humidity_percent")
    wind = optional_float(trail, "wind_speed_mph")
    profile_weather = {
        "temperature_f": temperature if temperature is not None else weather["temperature_f"],
        "humidity_percent": humidity if humidity is not None else weather["humidity_percent"],
        "wind_speed_mph": wind if wind is not None else weather["wind_speed_mph"],
    }
    weather_load = compute_weather_load(profile_weather)
    terrain_score = label_score(TERRAIN_SCORES, terrain)
    altitude_load = max(0.0, min(1.0, (altitude - 5280.0) / 2500.0))
    hr_load = max(0.0, min(1.0, 0.18 + min(grade / 1400.0, 1.0) * 0.34 + terrain_score * 0.22 + altitude_load * 0.14 + weather_load * 0.12))
    return {
        "distance_miles": distance,
        "elevation_gain_ft": gain,
        "gain_per_mile": grade,
        "average_elevation_ft": altitude,
        "terrain": terrain,
        "surface": surface,
        "terrain_score": terrain_score,
        "surface_score": label_score(SURFACE_SCORES, surface),
        "temperature_f": profile_weather["temperature_f"],
        "humidity_percent": profile_weather["humidity_percent"],
        "wind_speed_mph": profile_weather["wind_speed_mph"],
        "weather_load": weather_load,
        "hr_profile_load": hr_load,
    }


def run_profile(run: dict[str, Any]) -> dict[str, Any]:
    distance = required_float(run, "distance_miles")
    gain = required_float(run, "elevation_gain_ft")
    grade = gain_per_mile(gain, distance)
    terrain = normalize_label(optional_string(run, "terrain")) or infer_terrain(grade)
    surface = normalize_label(optional_string(run, "surface")) or infer_surface(terrain, grade)
    altitude = optional_float(run, "average_elevation_ft")
    if altitude is None:
        altitude = inferred_run_altitude(terrain, gain)
    weather = normalize_weather_profile(run)
    avg_hr = optional_float(run, "average_heartrate")
    max_hr = optional_float(run, "max_heartrate")
    perceived = optional_float(run, "perceived_effort")
    hr_load = compute_hr_load(avg_hr, max_hr, perceived)
    return {
        "distance_miles": distance,
        "elevation_gain_ft": gain,
        "gain_per_mile": grade,
        "average_elevation_ft": altitude,
        "terrain": terrain,
        "surface": surface,
        "terrain_score": label_score(TERRAIN_SCORES, terrain),
        "surface_score": label_score(SURFACE_SCORES, surface),
        "temperature_f": weather["temperature_f"],
        "humidity_percent": weather["humidity_percent"],
        "wind_speed_mph": weather["wind_speed_mph"],
        "weather_load": compute_weather_load(weather),
        "hr_profile_load": hr_load,
    }


def profile_scales(profiles: list[dict[str, Any]]) -> dict[str, float]:
    return {
        "distance_miles": value_range([required_float(profile, "distance_miles") for profile in profiles]),
        "elevation_gain_ft": value_range([required_float(profile, "elevation_gain_ft") for profile in profiles]),
        "gain_per_mile": value_range([required_float(profile, "gain_per_mile") for profile in profiles]),
        "average_elevation_ft": value_range([required_float(profile, "average_elevation_ft") for profile in profiles]),
    }


def similarity_components(profile: dict[str, Any], candidate: dict[str, Any], scales: dict[str, float]) -> dict[str, float]:
    vector = similarity_vector_delta(profile, candidate, scales)
    return {key: abs(value) for key, value in vector.items()}


def similarity_vector_delta(profile: dict[str, Any], candidate: dict[str, Any], scales: dict[str, float]) -> dict[str, float]:
    return {
        "distance": (required_float(candidate, "distance_miles") - required_float(profile, "distance_miles")) / scales["distance_miles"],
        "elevation_gain": (required_float(candidate, "elevation_gain_ft") - required_float(profile, "elevation_gain_ft")) / scales["elevation_gain_ft"],
        "grade": (required_float(candidate, "gain_per_mile") - required_float(profile, "gain_per_mile")) / scales["gain_per_mile"],
        "altitude": (required_float(candidate, "average_elevation_ft") - required_float(profile, "average_elevation_ft")) / scales["average_elevation_ft"],
        "terrain": required_float(candidate, "terrain_score") - required_float(profile, "terrain_score"),
        "surface": required_float(candidate, "surface_score") - required_float(profile, "surface_score"),
        "weather": required_float(candidate, "weather_load") - required_float(profile, "weather_load"),
        "hr_profile": required_float(candidate, "hr_profile_load") - required_float(profile, "hr_profile_load"),
    }


def similarity_vector(profile: dict[str, Any], scales: dict[str, float]) -> dict[str, float]:
    return {
        "distance": required_float(profile, "distance_miles") / scales["distance_miles"],
        "elevation_gain": required_float(profile, "elevation_gain_ft") / scales["elevation_gain_ft"],
        "grade": required_float(profile, "gain_per_mile") / scales["gain_per_mile"],
        "altitude": required_float(profile, "average_elevation_ft") / scales["average_elevation_ft"],
        "terrain": required_float(profile, "terrain_score"),
        "surface": required_float(profile, "surface_score"),
        "weather": required_float(profile, "weather_load"),
        "hr_profile": required_float(profile, "hr_profile_load"),
    }


def weighted_score(component_scores: dict[str, float]) -> float:
    return math.sqrt(sum(FEATURE_WEIGHTS[key] * component_scores[key] * component_scores[key] for key in FEATURE_WEIGHTS))


def weighted_feature_scores(component_scores: dict[str, float]) -> dict[str, float]:
    return {key: FEATURE_WEIGHTS[key] * component_scores[key] * component_scores[key] for key in FEATURE_WEIGHTS}


def public_profile(profile: dict[str, Any]) -> dict[str, Any]:
    return {
        "distance_miles": round(required_float(profile, "distance_miles"), 4),
        "elevation_gain_ft": round(required_float(profile, "elevation_gain_ft"), 2),
        "gain_per_mile": round(required_float(profile, "gain_per_mile"), 2),
        "average_elevation_ft": round(required_float(profile, "average_elevation_ft"), 2),
        "terrain": required_string(profile, "terrain"),
        "surface": required_string(profile, "surface"),
        "temperature_f": round(required_float(profile, "temperature_f"), 1),
        "humidity_percent": round(required_float(profile, "humidity_percent"), 1),
        "wind_speed_mph": round(required_float(profile, "wind_speed_mph"), 1),
        "weather_load": round(required_float(profile, "weather_load"), 4),
        "hr_profile_load": round(required_float(profile, "hr_profile_load"), 4),
    }


def explain_match(component_scores: dict[str, float], profile: dict[str, Any], candidate: dict[str, Any]) -> dict[str, Any]:
    ranked = sorted(component_scores.items(), key=lambda item: item[1])
    strongest = [name for name, score in ranked[:3] if score <= 0.35]
    gaps = [name for name, score in sorted(component_scores.items(), key=lambda item: item[1], reverse=True)[:2]]
    return {
        "strongest_features": strongest,
        "largest_gaps": gaps,
        "terrain_match": required_string(profile, "terrain") == required_string(candidate, "terrain"),
        "surface_match": required_string(profile, "surface") == required_string(candidate, "surface"),
        "summary": f"{required_string(candidate, 'terrain')} {required_string(candidate, 'surface')} run scored against {required_string(profile, 'terrain')} {required_string(profile, 'surface')} trail profile",
    }


def gain_per_mile(gain: float, distance: float) -> float:
    return gain / max(distance, 0.01)


def infer_terrain(gain_per_mile_value: float) -> str:
    if gain_per_mile_value >= 900:
        return "mountain"
    if gain_per_mile_value >= 450:
        return "hilly"
    if gain_per_mile_value >= 150:
        return "rolling"
    if gain_per_mile_value >= 25:
        return "path"
    return "flat"


def infer_surface(terrain: str, gain_per_mile_value: float) -> str:
    if terrain in {"mountain", "hilly", "rolling"} or gain_per_mile_value >= 150:
        return "trail"
    if terrain == "path":
        return "mixed"
    return "road"


def inferred_run_altitude(terrain: str, gain: float) -> float:
    base = {
        "flat": 5320.0,
        "road": 5320.0,
        "path": 5380.0,
        "rolling": 5700.0,
        "hilly": 5850.0,
        "mountain": 6350.0,
    }.get(terrain, 5500.0)
    return base + gain * 0.05


def compute_weather_load(weather: dict[str, float]) -> float:
    temp_penalty = abs(weather["temperature_f"] - 55.0) / 45.0
    humidity_penalty = max(weather["humidity_percent"] - 35.0, 0.0) / 65.0
    wind_penalty = weather["wind_speed_mph"] / 30.0
    return max(0.0, min(1.0, temp_penalty * 0.55 + humidity_penalty * 0.20 + wind_penalty * 0.25))


def compute_hr_load(avg_hr: float | None, max_hr: float | None, perceived_effort: float | None) -> float:
    values = []
    if avg_hr is not None:
        values.append(max(0.0, min(1.0, (avg_hr - 120.0) / 55.0)) * 0.55)
    if max_hr is not None:
        values.append(max(0.0, min(1.0, (max_hr - 145.0) / 45.0)) * 0.30)
    if perceived_effort is not None:
        values.append(max(0.0, min(1.0, perceived_effort / 10.0)) * 0.15)
    if not values:
        return 0.5
    return max(0.0, min(1.0, sum(values)))


def normalize_weather_profile(value: dict[str, Any] | None) -> dict[str, float]:
    if not value:
        return {key: float(DEFAULT_WEATHER_PROFILE[key]) for key in DEFAULT_WEATHER_PROFILE}
    source = value.get("current") if isinstance(value.get("current"), dict) else value
    return {
        "temperature_f": float(source.get("temperature_f", source.get("temperature_2m", DEFAULT_WEATHER_PROFILE["temperature_f"]))),
        "humidity_percent": float(source.get("humidity_percent", source.get("relative_humidity_2m", DEFAULT_WEATHER_PROFILE["humidity_percent"]))),
        "wind_speed_mph": float(source.get("wind_speed_mph", source.get("wind_speed_10m", DEFAULT_WEATHER_PROFILE["wind_speed_mph"]))),
    }


def label_score(scores: dict[str, float], value: str) -> float:
    return scores.get(normalize_label(value), 0.5)


def normalize_label(value: str | None) -> str:
    if not value:
        return ""
    return value.strip().lower().replace(" ", "_").replace("-", "_")


def round_dict(values: dict[str, float], digits: int) -> dict[str, float]:
    return {key: round(float(value), digits) for key, value in values.items()}


def value_range(values: list[float]) -> float:
    if not values:
        return 1.0
    return max(max(values) - min(values), 1.0)


def average(values: list[float]) -> float:
    if not values:
        raise ValueError("cannot average an empty list")
    return sum(values) / len(values)


def average_optional(values: list[Any]) -> float:
    numeric = [float(value) for value in values if value is not None]
    if not numeric:
        raise ValueError("cannot average an empty optional list")
    return average(numeric)


def required_string(row: dict[str, Any], key: str) -> str:
    value = row.get(key)
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"row is missing required string {key}")
    return value


def required_float(row: dict[str, Any], key: str) -> float:
    if key not in row or row[key] is None:
        raise ValueError(f"row is missing required number {key}")
    return float(row[key])


def optional_float(row: dict[str, Any], key: str) -> float | None:
    if row.get(key) is None:
        return None
    return float(row[key])


def optional_string(row: dict[str, Any], key: str) -> str | None:
    value = row.get(key)
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError(f"row field {key} must be a string")
    return value


def read_json(path: Path) -> Any:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--trails", type=Path, required=True, help="Trail rows JSON.")
    parser.add_argument("--runs", type=Path, required=True, help="RunActivity rows JSON.")
    parser.add_argument("--top-n", type=int, default=DEFAULT_TOP_N, help="Number of similar runs per trail.")
    parser.add_argument("--weather", type=Path, help="Optional current weather JSON used as the target weather profile.")
    parser.add_argument("--output", type=Path, help="Optional output JSON path.")
    args = parser.parse_args()
    weather = read_json(args.weather) if args.weather else None
    estimates = estimate_effort(read_json(args.trails), read_json(args.runs), args.top_n, weather_profile=weather)
    body = json.dumps(estimates, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(body, encoding="utf-8")
    else:
        print(body, end="")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"trail effort estimator failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
