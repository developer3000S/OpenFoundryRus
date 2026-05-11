#!/usr/bin/env python3
"""Run the Trail demo weather webhook/action contract against a selected trail."""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


def fetch_weather_snapshot(
    source: dict[str, Any],
    action: dict[str, Any],
    trail: dict[str, Any],
    base_url: str | None = None,
    mock_response: dict[str, Any] | None = None,
) -> dict[str, Any]:
    parameters = action_parameters_for_trail(trail)
    webhook_config = action["config"]["webhook_writeback"]
    webhook = source["config"]["webhook"]
    outputs = invoke_webhook(source, webhook, webhook_config, parameters, base_url, mock_response)
    alias = webhook_config.get("output_parameter_alias") or "webhook_output"
    parameters[alias] = outputs
    for mapping in webhook_config.get("output_mappings", []):
        output_name = mapping.get("webhook_output_name") or mapping.get("webhook_output_path")
        parameter_name = mapping.get("action_parameter_name") or mapping.get("action_input_name")
        if output_name and parameter_name and output_name in outputs:
            parameters[parameter_name] = outputs[output_name]
    parameters["weather_snapshot_id"] = snapshot_id(parameters["trail_id"], outputs["weather_time"])
    return apply_object_writeback(action, parameters)


def action_parameters_for_trail(trail: dict[str, Any]) -> dict[str, Any]:
    trail_id = required_string(trail, "trail_id")
    lat = required_float(trail, "start_lat")
    lon = required_float(trail, "start_lon")
    validate_latlon(lat, lon, f"trail {trail_id}")
    return {
        "trail_id": trail_id,
        "trail_name": required_string(trail, "trail_name"),
        "latitude": lat,
        "longitude": lon,
        "trailhead_geopoint": trail.get("trailhead_geopoint") or f"{lat},{lon}",
    }


def invoke_webhook(
    source: dict[str, Any],
    webhook: dict[str, Any],
    action_webhook_config: dict[str, Any],
    parameters: dict[str, Any],
    base_url: str | None,
    mock_response: dict[str, Any] | None,
) -> dict[str, Any]:
    inputs = {}
    for mapping in action_webhook_config.get("input_mappings", []):
        webhook_input = required_mapping_string(mapping, "webhook_input_name")
        action_input = required_mapping_string(mapping, "action_input_name")
        inputs[webhook_input] = lookup_path(parameters, action_input)
    validate_webhook_inputs(webhook, inputs)

    call = webhook["calls"][0]
    if mock_response is None:
        response = execute_webhook_call(source, call, inputs, base_url)
    else:
        response = mock_response
    return extract_outputs(webhook, response)


def execute_webhook_call(source: dict[str, Any], call: dict[str, Any], inputs: dict[str, Any], base_url: str | None) -> dict[str, Any]:
    method = call.get("method", "GET").upper()
    if method != "GET":
        raise ValueError("demo weather webhook only supports GET")
    root = (base_url or source["config"].get("base_url") or "").rstrip("/")
    if not root:
        root = "https://" + source["config"]["domain"].strip("/")
    query = {
        key: render_template(value, inputs)
        for key, value in call.get("query_params", {}).items()
    }
    url = root + call.get("path", "") + "?" + urllib.parse.urlencode(query)
    request = urllib.request.Request(url, method="GET", headers={"accept": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            body = response.read()
    except urllib.error.URLError as exc:
        raise ValueError(f"weather webhook request failed: {exc}") from exc
    return json.loads(body.decode("utf-8"))


def extract_outputs(webhook: dict[str, Any], response: dict[str, Any]) -> dict[str, Any]:
    outputs: dict[str, Any] = {}
    for output in webhook.get("outputs", []):
        output_id = output.get("id") or output.get("name")
        if not output_id:
            raise ValueError("webhook output requires id or name")
        path = output.get("extractor", {}).get("path")
        if not path:
            raise ValueError(f"webhook output {output_id} requires extractor.path")
        outputs[output_id] = json_pointer(response, path)
    return outputs


def apply_object_writeback(action: dict[str, Any], parameters: dict[str, Any]) -> dict[str, Any]:
    if action.get("operation_kind") not in {"update_object", "create_or_modify_object"}:
        raise ValueError(f"unsupported action operation_kind {action.get('operation_kind')}")
    snapshot: dict[str, Any] = {}
    for mapping in action["config"]["operation"].get("property_mappings", []):
        property_name = required_mapping_string(mapping, "property_name")
        if "input_name" in mapping:
            snapshot[property_name] = lookup_path(parameters, mapping["input_name"])
        elif "value" in mapping:
            snapshot[property_name] = mapping["value"]
        else:
            snapshot[property_name] = None
    snapshot["action_api_name"] = action["api_name"]
    snapshot["webhook_id"] = action["config"]["webhook_writeback"]["webhook_id"]
    return snapshot


def validate_webhook_inputs(webhook: dict[str, Any], inputs: dict[str, Any]) -> None:
    for input_def in webhook.get("inputs", []):
        input_id = input_def.get("id") or input_def.get("name")
        if input_def.get("required") and input_id not in inputs:
            raise ValueError(f"required webhook input {input_id} is missing")
        if input_id in inputs and input_def.get("type") == "number":
            required_float(inputs, input_id)


def render_template(value: str, inputs: dict[str, Any]) -> str:
    rendered = value
    for key, raw in inputs.items():
        rendered = rendered.replace("{{" + key + "}}", str(raw))
    return rendered


def json_pointer(value: Any, pointer: str) -> Any:
    if pointer == "" or pointer == "/":
        return value
    current = value
    for raw_segment in pointer.strip("/").split("/"):
        segment = raw_segment.replace("~1", "/").replace("~0", "~")
        if isinstance(current, list):
            current = current[int(segment)]
        elif isinstance(current, dict) and segment in current:
            current = current[segment]
        else:
            raise ValueError(f"JSON pointer {pointer} did not resolve at {segment}")
    return current


def lookup_path(value: dict[str, Any], path: str) -> Any:
    current: Any = value
    for segment in path.split("."):
        if not isinstance(current, dict) or segment not in current:
            raise ValueError(f"input path {path} did not resolve")
        current = current[segment]
    return current


def snapshot_id(trail_id: str, weather_time: str) -> str:
    safe_time = weather_time.replace(":", "-")
    return f"weather-{trail_id}-{safe_time}"


def required_mapping_string(row: dict[str, Any], key: str) -> str:
    value = row.get(key)
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"mapping field {key} must be a non-empty string")
    return value


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


def validate_latlon(lat: float, lon: float, label: str) -> None:
    if not -90.0 <= lat <= 90.0:
        raise ValueError(f"{label} latitude out of range")
    if not -180.0 <= lon <= 180.0:
        raise ValueError(f"{label} longitude out of range")


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def select_trail(path: Path, trail_id: str) -> dict[str, Any]:
    for row in read_json(path):
        if row.get("trail_id") == trail_id:
            return row
    raise ValueError(f"trail {trail_id} not found in {path}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path, help="REST API source contract JSON.")
    parser.add_argument("--action", required=True, type=Path, help="Action type contract JSON.")
    parser.add_argument("--trails", required=True, type=Path, help="Trail rows JSON.")
    parser.add_argument("--trail-id", required=True, help="Trail id to fetch weather for.")
    parser.add_argument("--base-url", help="Override source base URL, used by mocked HTTP tests.")
    parser.add_argument("--mock-response", type=Path, help="Use a saved Open-Meteo response instead of HTTP.")
    parser.add_argument("--output", type=Path, help="Optional WeatherSnapshot output JSON path.")
    args = parser.parse_args()

    snapshot = fetch_weather_snapshot(
        read_json(args.source),
        read_json(args.action),
        select_trail(args.trails, args.trail_id),
        base_url=args.base_url,
        mock_response=read_json(args.mock_response) if args.mock_response else None,
    )
    if args.output:
        write_json(args.output, snapshot)
    else:
        print(json.dumps(snapshot, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"weather action failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
