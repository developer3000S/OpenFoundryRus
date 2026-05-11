#!/usr/bin/env python3
"""Verify Palantir Foundry documentation links in a Markdown file.

This checker is intentionally narrow: it extracts external
https://www.palantir.com/docs/foundry links from the target Markdown file and
verifies that each unique URL returns a reachable HTTP status. Rate limits can
be allowlisted so CI still catches dead links without becoming brittle.
"""
from __future__ import annotations

import argparse
import concurrent.futures
import dataclasses
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Iterable

DEFAULT_DOC = Path("docs/migration/foundry-workshop-pipeline-1to1-checklist.md")
DEFAULT_PREFIX = "https://www.palantir.com/docs/foundry"
DEFAULT_TIMEOUT_SECONDS = 12.0
DEFAULT_WORKERS = 8
DEFAULT_ATTEMPTS = 2
DEFAULT_ALLOWED_STATUSES = "429"
USER_AGENT = "OpenFoundry-doc-contract/1.0"
URL_RE = re.compile(r"https://www\.palantir\.com/docs/foundry[^\s)>\]\"']*")


@dataclasses.dataclass(frozen=True)
class LinkCheck:
    url: str
    ok: bool
    status: int | None
    allowed: bool
    final_url: str | None
    error: str | None


def parse_statuses(value: str) -> set[int]:
    statuses: set[int] = set()
    for part in value.split(","):
        part = part.strip()
        if not part:
            continue
        try:
            statuses.add(int(part))
        except ValueError as exc:
            raise SystemExit(f"invalid HTTP status in allowlist: {part!r}") from exc
    return statuses


def normalize_url(raw: str) -> str:
    return raw.rstrip(".,;:")


def extract_links(path: Path, prefix: str) -> list[str]:
    text = path.read_text(encoding="utf-8")
    urls = {normalize_url(match.group(0)) for match in URL_RE.finditer(text)}
    return sorted(url for url in urls if url.startswith(prefix))


def make_request(url: str, method: str, timeout: float) -> tuple[int, str]:
    request = urllib.request.Request(
        url,
        method=method,
        headers={
            "Accept": "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9",
            "Range": "bytes=0-4096",
            "User-Agent": USER_AGENT,
        },
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        if method == "GET":
            response.read(4096)
        return response.status, response.geturl()


def fetch_once(url: str, timeout: float) -> tuple[int | None, str | None, str | None]:
    last_status: int | None = None
    final_url: str | None = None
    last_error: str | None = None

    for method in ("HEAD", "GET"):
        try:
            status, final_url = make_request(url, method, timeout)
            return status, final_url, None
        except urllib.error.HTTPError as exc:
            last_status = exc.code
            final_url = exc.geturl()
            last_error = f"HTTP {exc.code}"
            if method == "HEAD":
                continue
            return last_status, final_url, last_error
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            last_error = str(exc)
            if method == "HEAD":
                continue
            return last_status, final_url, last_error

    return last_status, final_url, last_error


def check_link(url: str, timeout: float, attempts: int, allowed_statuses: set[int]) -> LinkCheck:
    last_status: int | None = None
    last_final_url: str | None = None
    last_error: str | None = None

    for attempt in range(max(attempts, 1)):
        status, final_url, error = fetch_once(url, timeout)
        last_status = status
        last_final_url = final_url
        last_error = error

        if status is not None and 200 <= status < 400:
            return LinkCheck(url, True, status, False, final_url, None)
        if status in allowed_statuses:
            return LinkCheck(url, True, status, True, final_url, error)
        if attempt < attempts - 1:
            time.sleep(0.5 * (attempt + 1))

    return LinkCheck(url, False, last_status, False, last_final_url, last_error)


def check_links(
    urls: Iterable[str],
    timeout: float,
    attempts: int,
    allowed_statuses: set[int],
    workers: int,
) -> list[LinkCheck]:
    with concurrent.futures.ThreadPoolExecutor(max_workers=max(workers, 1)) as executor:
        futures = [
            executor.submit(check_link, url, timeout, attempts, allowed_statuses)
            for url in urls
        ]
        return [future.result() for future in concurrent.futures.as_completed(futures)]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "markdown_file",
        nargs="?",
        default=str(DEFAULT_DOC),
        help=f"Markdown file to scan, default: {DEFAULT_DOC}",
    )
    parser.add_argument(
        "--prefix",
        default=DEFAULT_PREFIX,
        help=f"Only URLs with this prefix are checked, default: {DEFAULT_PREFIX}",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=DEFAULT_TIMEOUT_SECONDS,
        help=f"HTTP timeout per request in seconds, default: {DEFAULT_TIMEOUT_SECONDS}",
    )
    parser.add_argument(
        "--attempts",
        type=int,
        default=DEFAULT_ATTEMPTS,
        help=f"Attempts per URL, default: {DEFAULT_ATTEMPTS}",
    )
    parser.add_argument(
        "--workers",
        type=int,
        default=DEFAULT_WORKERS,
        help=f"Concurrent workers, default: {DEFAULT_WORKERS}",
    )
    parser.add_argument(
        "--allow-status",
        default=os.environ.get("OF_DOC_CONTRACT_ALLOWED_STATUSES", DEFAULT_ALLOWED_STATUSES),
        help=(
            "Comma-separated HTTP statuses treated as non-fatal, "
            f"default: {DEFAULT_ALLOWED_STATUSES}"
        ),
    )
    args = parser.parse_args()

    markdown_path = Path(args.markdown_file)
    if not markdown_path.exists():
        print(f"error: file not found: {markdown_path}", file=sys.stderr)
        return 2

    allowed_statuses = parse_statuses(args.allow_status)
    urls = extract_links(markdown_path, args.prefix)
    if not urls:
        print(f"error: no Palantir Foundry documentation links found in {markdown_path}", file=sys.stderr)
        return 1

    print(f"Checking {len(urls)} Palantir Foundry documentation links from {markdown_path}")
    results = sorted(
        check_links(urls, args.timeout, args.attempts, allowed_statuses, args.workers),
        key=lambda result: result.url,
    )

    failures = [result for result in results if not result.ok]
    for result in results:
        status = result.status if result.status is not None else "network"
        suffix = " allowed" if result.allowed else ""
        if result.ok:
            print(f"ok {status}{suffix} {result.url}")
        else:
            detail = f" ({result.error})" if result.error else ""
            print(f"fail {status} {result.url}{detail}", file=sys.stderr)

    allowed = [result for result in results if result.allowed]
    if allowed:
        print(
            f"warning: {len(allowed)} URL(s) returned allowlisted statuses: "
            + ", ".join(str(result.status) for result in allowed)
        )

    if failures:
        print(f"error: {len(failures)} Palantir documentation link(s) failed", file=sys.stderr)
        return 1

    print("All Palantir Foundry documentation links are reachable")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
