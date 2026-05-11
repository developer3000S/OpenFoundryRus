#!/usr/bin/env python3
"""Run the Workshop custom GPX upload flow against local demo fixtures."""

from __future__ import annotations

import argparse
import importlib.util
import json
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
GPX_PIPELINE = ROOT / "pipelines" / "gpx_trail_ingestion.py"
EFFORT_FUNCTION = ROOT / "functions" / "effort_estimator.py"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gpx", type=Path, required=True, help="GPX file to parse.")
    parser.add_argument("--runs", type=Path, required=True, help="Normalized run activity JSON file.")
    parser.add_argument("--trail-output", type=Path, required=True, help="Where to write parsed Trail rows.")
    parser.add_argument("--estimate-output", type=Path, required=True, help="Where to write TrailEffortEstimate rows.")
    parser.add_argument("--top-n", type=int, default=5, help="Similar run count for effort estimation.")
    args = parser.parse_args()

    gpx_path = args.gpx.resolve()
    runs_path = args.runs.resolve()
    source_name = source_name_for(gpx_path)

    gpx_ingestion = load_module("trail_running_gpx_upload_ingestion", GPX_PIPELINE)
    effort_estimator = load_module("trail_running_gpx_upload_effort", EFFORT_FUNCTION)

    trails = gpx_ingestion.normalize_gpx_files([gpx_path], [source_name])
    if len(trails) != 1:
        raise ValueError(f"expected one Trail row from {gpx_path}, got {len(trails)}")

    runs = read_json(runs_path)
    estimates = effort_estimator.estimate_effort(trails, runs, top_n=args.top_n)
    if len(estimates) != 1:
        raise ValueError(f"expected one TrailEffortEstimate row, got {len(estimates)}")

    write_json(args.trail_output.resolve(), trails)
    write_json(args.estimate_output.resolve(), estimates)
    return 0


def source_name_for(gpx_path: Path) -> str:
    fixtures = ROOT / "fixtures"
    try:
        return gpx_path.relative_to(fixtures).as_posix()
    except ValueError:
        return gpx_path.name


def load_module(module_name: str, path: Path) -> Any:
    spec = importlib.util.spec_from_file_location(module_name, path)
    if spec is None or spec.loader is None:
        raise ValueError(f"unable to load module from {path}")
    module = importlib.util.module_from_spec(spec)
    old_dont_write_bytecode = sys.dont_write_bytecode
    sys.dont_write_bytecode = True
    try:
        spec.loader.exec_module(module)
    finally:
        sys.dont_write_bytecode = old_dont_write_bytecode
    return module


def read_json(path: Path) -> Any:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as handle:
        json.dump(value, handle, indent=2, sort_keys=True)
        handle.write("\n")


if __name__ == "__main__":
    raise SystemExit(main())
