#!/usr/bin/env python3
"""Extract and compare Rust Axum and Go chi HTTP route tables.

This is intentionally heuristic: it targets the routing styles used in this
repository (Axum `.route(... get(handler).post(handler))` and chi
`r.Route(...); r.Get/Post/...`). It emits a migration-oriented Markdown report,
not a formal OpenAPI document.
"""
from __future__ import annotations

import argparse
import dataclasses
import re
from pathlib import Path
from typing import Iterable

DEFAULT_SERVICES = [
    "pipeline-build-service",
    "notebook-runtime-service",
    "ontology-query-service",
    "connector-management-service",
    "dataset-versioning-service",
    "ingestion-replication-service",
    "iceberg-catalog-service",
    "federation-product-exchange-service",
]

METHODS = ("get", "post", "put", "patch", "delete", "head", "options")
GO_METHODS = {
    "Get": "GET",
    "Post": "POST",
    "Put": "PUT",
    "Patch": "PATCH",
    "Delete": "DELETE",
    "Head": "HEAD",
    "Options": "OPTIONS",
}

@dataclasses.dataclass(frozen=True)
class Route:
    service: str
    side: str
    method: str
    path: str
    handler: str
    file: str
    line: int
    status: str = "unknown"


def rel(path: Path, root: Path) -> str:
    try:
        return str(path.relative_to(root))
    except ValueError:
        return str(path)


def normalize_path(path: str) -> str:
    path = re.sub(r"\{([^}:]+):[^}]+\}", r"{\1}", path)
    path = re.sub(r"<([^>]+)>", r"{\1}", path)
    path = re.sub(r"/{2,}", "/", path)
    return path.rstrip("/") if path != "/" else path


def join_paths(prefix: str, path: str) -> str:
    if not prefix:
        return normalize_path(path)
    return normalize_path(prefix.rstrip("/") + "/" + path.lstrip("/"))


def iter_files(root: Path, suffix: str) -> Iterable[Path]:
    if not root.exists():
        return []
    return sorted(p for p in root.rglob(f"*{suffix}") if p.is_file())


def find_matching(text: str, open_idx: int, open_ch: str = "(", close_ch: str = ")") -> int:
    depth = 0
    in_str = False
    escaped = False
    for i in range(open_idx, len(text)):
        ch = text[i]
        if in_str:
            if escaped:
                escaped = False
            elif ch == "\\":
                escaped = True
            elif ch == '"':
                in_str = False
            continue
        if ch == '"':
            in_str = True
        elif ch == open_ch:
            depth += 1
        elif ch == close_ch:
            depth -= 1
            if depth == 0:
                return i
    return -1


def line_no(text: str, idx: int) -> int:
    return text.count("\n", 0, idx) + 1


def split_top_level_args(text: str) -> list[str]:
    args: list[str] = []
    start = 0
    depth = 0
    in_str = False
    escaped = False
    for i, ch in enumerate(text):
        if in_str:
            if escaped:
                escaped = False
            elif ch == "\\":
                escaped = True
            elif ch == '"':
                in_str = False
            continue
        if ch == '"':
            in_str = True
        elif ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        elif ch == "," and depth == 0:
            args.append(text[start:i].strip())
            start = i + 1
    tail = text[start:].strip()
    if tail:
        args.append(tail)
    return args



def find_statement_semicolon(text: str, start_idx: int) -> int:
    in_str = False
    escaped = False
    in_line_comment = False
    for i in range(start_idx, len(text)):
        ch = text[i]
        nxt = text[i + 1] if i + 1 < len(text) else ""
        if in_line_comment:
            if ch == "\n":
                in_line_comment = False
            continue
        if in_str:
            if escaped:
                escaped = False
            elif ch == "\\":
                escaped = True
            elif ch == '"':
                in_str = False
            continue
        if ch == "/" and nxt == "/":
            in_line_comment = True
            continue
        if ch == '"':
            in_str = True
            continue
        if ch == ";":
            return i
    return -1

def rust_router_blocks(text: str) -> dict[tuple[int, int], str]:
    blocks: dict[tuple[int, int], str] = {}
    for m in re.finditer(r"let\s+(\w+)\s*=\s*Router::new\s*\(\s*\)", text):
        semi = find_statement_semicolon(text, m.end())
        if semi != -1:
            blocks[(m.start(), semi)] = m.group(1)
    return blocks


def rust_prefixes(text: str) -> dict[str, str]:
    prefixes: dict[str, str] = {}
    for m in re.finditer(r"\.nest\s*\(\s*\"([^\"]+)\"\s*,\s*(\w+)\s*\)", text):
        prefixes[m.group(2)] = m.group(1)
    return prefixes


def extract_rust_routes(repo: Path, service: str) -> list[Route]:
    root = repo / "services" / service / "src"
    routes: list[Route] = []
    for path in iter_files(root, ".rs"):
        text = path.read_text(errors="ignore")
        blocks = rust_router_blocks(text)
        prefixes = rust_prefixes(text)
        for m in re.finditer(r"\.route\s*\(", text):
            open_idx = text.find("(", m.start())
            close_idx = find_matching(text, open_idx)
            if close_idx == -1:
                continue
            args = split_top_level_args(text[open_idx + 1:close_idx])
            if len(args) < 2:
                continue
            route_match = re.match(r'"([^"]+)"', args[0])
            if not route_match:
                continue
            path_part = route_match.group(1)
            router_name = None
            for (start, end), name in blocks.items():
                if start <= m.start() <= end:
                    router_name = name
                    break
            full_path = join_paths(prefixes.get(router_name or "", ""), path_part)
            handlers_expr = args[1]
            found = False
            for meth in METHODS:
                pattern = rf"(?:axum::routing::)?{meth}\s*\(\s*([^\)\s,]+)"
                for hm in re.finditer(pattern, handlers_expr):
                    routes.append(Route(service, "rust", meth.upper(), full_path, hm.group(1), rel(path, repo), line_no(text, m.start())))
                    found = True
            if not found and "MethodRouter" in handlers_expr:
                routes.append(Route(service, "rust", "ANY", full_path, handlers_expr.strip(), rel(path, repo), line_no(text, m.start())))
    return routes


def extract_go_function_bodies(service_root: Path) -> dict[str, list[str]]:
    bodies: dict[str, list[str]] = {}
    for path in iter_files(service_root, ".go"):
        text = path.read_text(errors="ignore")
        for m in re.finditer(r"func\s+(?:\([^\)]*\)\s*)?(\w+)\s*\(", text):
            brace = text.find("{", m.end())
            if brace == -1:
                continue
            close = find_matching(text, brace, "{", "}")
            if close != -1:
                bodies.setdefault(m.group(1), []).append(text[brace + 1:close])
    return bodies


def classify_go_handler(handler: str, bodies: dict[str, list[str]]) -> str:
    name = handler.split(".")[-1]
    candidates = bodies.get(name, [])
    if not candidates:
        return "implemented"
    # Prefer the most conservative migration state when multiple packages have
    # a same-named helper/method (for example a handler and a domain function).
    saw_empty = False
    for body in candidates:
        compact = re.sub(r"\s+", " ", body)
        if "http.StatusNotImplemented" in body or "notImplemented(w" in body:
            return "501"
        if "StatusServiceUnavailable" in body and "pending" in body.lower():
            return "501"
        if "writeEmptyList" in body or '"data": []any{}' in compact or "[]any{}" in compact:
            saw_empty = True
    if saw_empty:
        return "empty envelope"
    return "implemented"


def extract_go_routes(repo: Path, service: str) -> list[Route]:
    root = repo / "openfoundry-go" / "services" / service
    routes: list[Route] = []
    bodies = extract_go_function_bodies(root)
    route_start = re.compile(r"(\w+)\.Route\s*\(\s*\"([^\"]+)\"\s*,\s*func\s*\(\s*(\w+)\s+chi\.Router\s*\)")
    direct = re.compile(r"(\w+)\.(Get|Post|Put|Patch|Delete|Head|Options)\s*\(\s*\"([^\"]+)\"\s*,\s*([^\)\s,]+)")
    method_call = re.compile(r"(\w+)\.Method\s*\(\s*http\.Method(\w+)\s*,\s*\"([^\"]+)\"\s*,\s*([^\)\s,]+)")
    for path in iter_files(root, ".go"):
        text = path.read_text(errors="ignore")
        var_prefix: dict[str, str] = {"r": ""}
        stack: list[tuple[str, int]] = []
        for no, line in enumerate(text.splitlines(), 1):
            m = route_start.search(line)
            if m:
                parent, prefix, child = m.groups()
                var_prefix[child] = join_paths(var_prefix.get(parent, ""), prefix)
                stack.append((child, line.count("{") - line.count("}")))
                continue
            for sm in stack:
                pass
            m2 = direct.search(line)
            if m2:
                var, meth, path_part, handler = m2.groups()
                full_path = join_paths(var_prefix.get(var, ""), path_part)
                routes.append(Route(service, "go", GO_METHODS[meth], full_path, handler, rel(path, repo), no, classify_go_handler(handler, bodies)))
            m3 = method_call.search(line)
            if m3:
                var, meth, path_part, handler = m3.groups()
                full_path = join_paths(var_prefix.get(var, ""), path_part)
                routes.append(Route(service, "go", meth.upper(), full_path, handler, rel(path, repo), no, classify_go_handler(handler, bodies)))
            # Pop chi.Route blocks when braces close. This intentionally only
            # tracks simple one-line func starts, which is how server routes are
            # declared in the Go services.
            if stack:
                child, depth = stack[-1]
                depth += line.count("{") - line.count("}")
                if depth <= 0:
                    stack.pop()
                    var_prefix.pop(child, None)
                else:
                    stack[-1] = (child, depth)
    return routes


def comparable_path(path: str) -> str:
    # URL parameter *names* are implementation details for both Axum and chi.
    # Compare structural parity on placeholder position while preserving the
    # original path in the report table.
    return re.sub(r"\{[^}]+\}", "{}", normalize_path(path))


def route_key(route: Route) -> tuple[str, str]:
    return (route.method, comparable_path(route.path))


def report_for_service(repo: Path, service: str) -> str:
    rust = extract_rust_routes(repo, service)
    go = extract_go_routes(repo, service)
    rust_by = {route_key(r): r for r in rust}
    go_by = {route_key(g): g for g in go}
    keys = sorted(set(rust_by) | set(go_by), key=lambda k: (k[1], k[0]))
    lines = [f"## {service}", "", f"Rust routes: {len(rust)}. Go routes: {len(go)}.", "", "| Route | Method | Rust handler | Go handler | State |", "| --- | --- | --- | --- | --- |"]
    state_counts: dict[str, int] = {}
    rows: list[str] = []
    for method, path in keys:
        rr = rust_by.get((method, path))
        gg = go_by.get((method, path))
        display_path = rr.path if rr else (gg.path if gg else path)
        if gg:
            state = gg.status
        else:
            state = "missing"
        state_counts[state] = state_counts.get(state, 0) + 1
        rust_handler = f"`{rr.handler}`<br><sub>{rr.file}:{rr.line}</sub>" if rr else "—"
        go_handler = f"`{gg.handler}`<br><sub>{gg.file}:{gg.line}</sub>" if gg else "—"
        rows.append(f"| `{display_path}` | {method} | {rust_handler} | {go_handler} | {state} |")
    counts = ", ".join(f"{k}: {state_counts[k]}" for k in sorted(state_counts))
    lines.insert(3, f"State counts: {counts}.")
    lines.extend(rows)
    lines.append("")
    return "\n".join(lines)


def generate_report(repo: Path, services: list[str]) -> str:
    parts = [
        "# Rust vs Go HTTP route parity audit",
        "",
        "Date: 2026-05-07",
        "",
        "Generated with:", "",
        "```sh",
        "python3 tools/http_route_audit.py --write openfoundry-go/HTTP-ROUTE-AUDIT.md",
        "```",
        "",
        "State values:", "",
        "- `implemented`: route exists in Rust and Go and the Go handler is not detected as a placeholder.",
        "- `empty envelope`: Go route exists but returns a placeholder empty/list envelope.",
        "- `501`: Go route exists but the handler advertises Not Implemented or equivalent pending behavior.",
        "- `missing`: Rust route was not found in Go. A blank Rust handler (`—`) means the Go route was not found in the Rust route table (usually health/metrics aliases or newer Go-only surface).",
        "",
        "This script is regex-based and optimized for the Axum/chi route declaration styles used in this repository; validate unusual dynamic route construction manually.",
        "",
    ]
    for service in services:
        parts.append(report_for_service(repo, service))
    return "\n".join(parts).rstrip() + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, default=Path.cwd())
    parser.add_argument("--services", nargs="*", default=DEFAULT_SERVICES)
    parser.add_argument("--write", type=Path)
    args = parser.parse_args()
    report = generate_report(args.repo.resolve(), args.services)
    if args.write:
        args.write.write_text(report)
    else:
        print(report)
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
