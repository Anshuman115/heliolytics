#!/usr/bin/env python3
"""Fail on skipped tests, failures, zero-test packages and unexpected no-test packages."""

import json
import sys
from collections import defaultdict

ALLOWED_NO_TEST = {"github.com/Anshuman115/heliolytics/internal/store/db"}

if len(sys.argv) != 2:
    print("usage: go-test-check.py <go-test-jsonl>", file=sys.stderr)
    sys.exit(2)
path = sys.argv[1]
problems = []
passed = defaultdict(int)
skipped = defaultdict(list)
failed = defaultdict(list)
terminal = {}
no_test = set()
bad_lines = 0

with open(path, encoding="utf-8") as fh:
    for line in fh:
        line = line.strip()
        if not line:
            continue
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            bad_lines += 1
            continue
        pkg = ev.get("Package", "")
        act = ev.get("Action")
        test = ev.get("Test")
        if test:
            if act == "pass":
                passed[pkg] += 1
            elif act == "skip":
                skipped[pkg].append(test)
            elif act == "fail":
                failed[pkg].append(test)
        else:
            if act in ("pass", "fail", "skip"):
                terminal[pkg] = act
                if act == "skip":
                    no_test.add(pkg)
            if act == "output" and "[no test files]" in ev.get("Output", ""):
                no_test.add(pkg)

if bad_lines:
    problems.append(f"{bad_lines} non-JSON lines (build output or truncation)")
for pkg, tests in sorted(skipped.items()):
    problems.append(f"SKIP {pkg}: {', '.join(tests)}")
for pkg, tests in sorted(failed.items()):
    problems.append(f"FAIL {pkg}: {', '.join(tests)}")
for pkg, act in sorted(terminal.items()):
    if act == "fail" and pkg not in failed:
        problems.append(f"FAIL {pkg}: package failed (build or setup)")
for pkg in sorted(no_test - ALLOWED_NO_TEST):
    problems.append(f"NO-TEST package not allowed: {pkg}")
for pkg, act in sorted(terminal.items()):
    if pkg in no_test:
        continue
    if passed[pkg] == 0:
        problems.append(f"ZERO passed tests in {pkg} (terminal {act})")

total = 0
for pkg in sorted(terminal):
    tag = "no-test" if pkg in no_test else f"{passed[pkg]} passed"
    print(f"{terminal[pkg]:5} {pkg} {tag}")
    total += passed[pkg]
print(f"packages={len(terminal)} passed_tests={total} skipped={sum(len(v) for v in skipped.values())}")
if problems:
    print("PROBLEMS:")
    for p in problems:
        print("  " + p)
    sys.exit(1)
print("OK")
