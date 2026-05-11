"""Three execution paths invoked by the gRPC servicer.

Each function captures stdout (and stderr where the Rust path did) using
``contextlib.redirect_stdout`` to mirror the Rust ``io.StringIO`` trick
byte-for-byte. The user script runs in a fresh ``exec`` namespace that
the runtime later inspects for sentinel variables (``result``,
``object_patch``, ``link``, ``delete_object``, ``rows_affected``,
``result_rows``).
"""

from __future__ import annotations

import contextlib
import io
import json
import os
import traceback
from typing import Any
from uuid import UUID

from .bootstrap import make_globals
from .sessions import SessionRegistry


class ExecutionError(Exception):
    """Raised when the user code fails. Wrapped into the response."""


def execute_inline_function(source: str, input_json: str) -> dict[str, Any]:
    """Run an ontology inline Python function.

    Returns a dict with ``result_json``, ``stdout``, ``stderr``, ``error``.
    Mirrors the Rust ``execute_inline_python_function`` shape — the
    returned ``result_json`` is the full enriched envelope the Go caller
    will pass back to the HTTP layer.
    """
    try:
        envelope = json.loads(input_json) if input_json else {}
    except json.JSONDecodeError as error:
        return _failure(f"invalid input_json: {error}")

    globals_dict = make_globals(envelope)
    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()

    try:
        with contextlib.redirect_stdout(stdout_buf), contextlib.redirect_stderr(stderr_buf):
            exec(compile(source, "<inline-function>", "exec"), globals_dict)
    except Exception:  # noqa: BLE001 — user code can raise anything
        return _failure(traceback.format_exc(), stdout=stdout_buf.getvalue(), stderr=stderr_buf.getvalue())

    payload: dict[str, Any] = {}
    for key, sentinel in (
        ("result", "result"),
        ("object_patch", "object_patch"),
        ("link", "link"),
        ("delete_object", "delete_object"),
    ):
        value = globals_dict.get(sentinel)
        if value is not None:
            payload[key] = value

    payload.setdefault("output", payload.get("result"))
    stdout = stdout_buf.getvalue()
    stderr = stderr_buf.getvalue()
    if stdout:
        payload["stdout"] = stdout
    if stderr:
        payload["stderr"] = stderr

    return {
        "result_json": json.dumps(payload, default=_json_default),
        "stdout": stdout,
        "stderr": stderr,
        "error": "",
    }


def execute_pipeline_transform(
    source: str,
    config_json: str,
    prepared_inputs_json: str,
    input_dataset_ids: list[str],
    output_dataset_id: str,
) -> dict[str, Any]:
    """Run a pipeline node Python transform.

    Mirrors the locals injected by the Rust ``execute_python_transform``
    path (services/pipeline-build-service/src/domain/engine/runtime.rs
    around line 385). Returns ``rows_affected``, ``output_json``,
    ``result_rows_json``, ``stdout``, ``error``.
    """
    try:
        config = json.loads(config_json) if config_json else {}
        prepared_inputs = json.loads(prepared_inputs_json) if prepared_inputs_json else []
    except json.JSONDecodeError as error:
        return _pipeline_failure(f"invalid request json: {error}")

    globals_dict: dict[str, Any] = {
        "__builtins__": __builtins__,
        "json": json,
        "config": config,
        "prepared_inputs": prepared_inputs,
        "input_datasets": prepared_inputs,
        "input_rows": prepared_inputs[0].get("rows", []) if prepared_inputs else [],
        "input_dataset_ids": list(input_dataset_ids),
        "output_dataset_id": output_dataset_id or None,
    }
    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()

    try:
        with contextlib.redirect_stdout(stdout_buf), contextlib.redirect_stderr(stderr_buf):
            exec(compile(source, "<pipeline-transform>", "exec"), globals_dict)
    except Exception:  # noqa: BLE001
        return _pipeline_failure(traceback.format_exc(), stdout=stdout_buf.getvalue(), stderr=stderr_buf.getvalue())

    rows_affected_raw = globals_dict.get("rows_affected")
    rows_affected_set = rows_affected_raw is not None
    rows_affected = int(rows_affected_raw) if rows_affected_set else 0

    result_value = globals_dict.get("result")
    result_str = str(result_value) if result_value is not None else None

    raw_rows = globals_dict.get("result_rows")
    if raw_rows is None:
        result_rows_json = ""
        normalized_rows: list[Any] | None = None
    else:
        if isinstance(raw_rows, dict):
            normalized_rows = [raw_rows]
        elif isinstance(raw_rows, list):
            normalized_rows = raw_rows
        else:
            return _pipeline_failure("result_rows must be an object or list of objects")
        if not rows_affected_set:
            rows_affected = len(normalized_rows)
            rows_affected_set = True
        result_rows_json = json.dumps(normalized_rows, default=_json_default)

    stdout = stdout_buf.getvalue()
    stderr = stderr_buf.getvalue()
    output_payload = {
        "stdout": stdout,
        "stderr": stderr,
        "result": result_str,
        "sample_rows": normalized_rows[:10] if normalized_rows else None,
    }

    return {
        "rows_affected": rows_affected,
        "rows_affected_set": rows_affected_set,
        "output_json": json.dumps(output_payload, default=_json_default),
        "result_rows_json": result_rows_json,
        "stdout": stdout,
        "error": "",
    }


def execute_notebook_cell(
    sessions: SessionRegistry,
    session_id: UUID | None,
    source: str,
    workspace_dir: str,
    notebook_id: UUID | None,
) -> dict[str, Any]:
    """Run one notebook cell.

    Mirrors `services/notebook-runtime-service/src/domain/kernel/python.rs`.
    When ``session_id`` is provided the same globals dict is reused
    across calls, persisting ``import``s and assigned variables.
    """
    if session_id is not None:
        globals_dict = sessions.ensure(session_id)
    else:
        globals_dict = {"__builtins__": __builtins__}

    workspace = workspace_dir or ""
    notebook_id_str = str(notebook_id) if notebook_id is not None else ""

    setup = (
        "import io, sys, os, pathlib\n"
        f"workspace_dir = {workspace!r}\n"
        f"notebook_id = {notebook_id_str!r}\n"
        "def workspace_path(*parts):\n"
        "    base = pathlib.Path(workspace_dir) if workspace_dir else pathlib.Path.cwd()\n"
        "    return str(base.joinpath(*parts))\n"
        "os.environ['OPENFOUNDRY_NOTEBOOK_ID'] = notebook_id\n"
        "os.environ['OPENFOUNDRY_NOTEBOOK_WORKSPACE'] = workspace_dir\n"
    )
    try:
        exec(compile(setup, "<notebook-setup>", "exec"), globals_dict)
    except Exception:  # noqa: BLE001
        return {
            "output_type": "error",
            "content_json": json.dumps(""),
            "stdout": "",
            "error": f"setup error: {traceback.format_exc()}",
        }

    stdout_buf = io.StringIO()
    try:
        with contextlib.redirect_stdout(stdout_buf):
            exec(compile(source, "<notebook-cell>", "exec"), globals_dict)
    except Exception:  # noqa: BLE001
        return {
            "output_type": "error",
            "content_json": json.dumps(stdout_buf.getvalue()),
            "stdout": stdout_buf.getvalue(),
            "error": traceback.format_exc(),
        }

    stdout = stdout_buf.getvalue()
    return {
        "output_type": "text",
        "content_json": json.dumps(stdout),
        "stdout": stdout,
        "error": "",
    }


def _failure(message: str, *, stdout: str = "", stderr: str = "") -> dict[str, Any]:
    return {"result_json": "", "stdout": stdout, "stderr": stderr, "error": message}


def _pipeline_failure(message: str, *, stdout: str = "", stderr: str = "") -> dict[str, Any]:
    if stderr:
        message = f"{message}\nstderr:\n{stderr}"
    return {
        "rows_affected": 0,
        "rows_affected_set": False,
        "output_json": "",
        "result_rows_json": "",
        "stdout": stdout,
        "error": message,
    }


def _json_default(value: Any) -> Any:
    """Coerce non-JSON-native types to strings, matching Rust serde best-effort."""
    if isinstance(value, (set, frozenset, tuple)):
        return list(value)
    if isinstance(value, bytes):
        return value.decode("utf-8", "replace")
    if hasattr(value, "isoformat"):
        return value.isoformat()
    return str(value)


# Silence the unused-import warning for os (kept for namespace stability).
_ = os
