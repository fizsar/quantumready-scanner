#!/usr/bin/env python3
"""Tests for check_base_pins.py with a fake docker (no images pulled)."""
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import check_base_pins as c  # noqa: E402

BASE = "nginxinc/nginx-unprivileged:1.30-alpine-slim@sha256:" + "a" * 64

DOCKERFILE = f"""FROM node:24@sha256:{'b' * 64} AS build
RUN apk add --no-cache 'git>=9.9-r0'   # base-pins (build stage: ignored)
FROM {BASE}
USER root
RUN apk add --no-cache 'musl>=1.2.6-r3' 'musl-utils>=1.2.6-r3' 'zlib>=1.3.2-r1'   # base-pins
USER 101
"""


def ver_key(v):
    # Enough of apk's ordering for the test: numbers, then the -rN release.
    main, rel = v.rsplit("-r", 1)
    return [int(x) for x in main.split(".")] + [int(rel)]


def fake_docker(packages):
    def docker(args):
        assert args[:5] == ["docker", "run", "--rm", "--entrypoint", "apk"], args
        assert args[5] == BASE, args
        if args[6:] == ["info", "-v"]:
            return "\n".join(f"{k}-{v}" for k, v in packages.items()) + "\n"
        if args[6:8] == ["version", "-t"]:
            a, b = ver_key(args[8]), ver_key(args[9])
            return "<\n" if a < b else "=\n" if a == b else ">\n"
        raise AssertionError(args)

    return docker


class CheckBasePins(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".Dockerfile")
        with os.fdopen(fd, "w") as f:
            f.write(DOCKERFILE)

    def tearDown(self):
        os.unlink(self.path)

    def test_parse_reads_the_final_stage_only(self):
        base, pins = c.parse(self.path)
        self.assertEqual(base, BASE)
        self.assertEqual(pins, {"musl": "1.2.6-r3", "musl-utils": "1.2.6-r3", "zlib": "1.3.2-r1"})

    def test_still_needed_when_the_base_is_older(self):
        code, lines = c.check(self.path, fake_docker({"musl": "1.2.6-r2", "musl-utils": "1.2.6-r2", "zlib": "1.3.2-r0", "busybox": "1.37.0-r1"}))
        self.assertEqual(code, 0, lines)
        self.assertIn("musl: the base has 1.2.6-r2 < 1.2.6-r3: pin still needed", lines)

    def test_still_needed_if_only_one_is_missing(self):
        code, lines = c.check(self.path, fake_docker({"musl": "1.2.6-r3", "musl-utils": "1.2.6-r3", "zlib": "1.3.2-r0"}))
        self.assertEqual(code, 0, lines)

    def test_fails_once_the_base_carries_every_minimum(self):
        code, lines = c.check(self.path, fake_docker({"musl": "1.2.6-r3", "musl-utils": "1.2.6-r4", "zlib": "1.4-r0"}))
        self.assertEqual(code, 1, lines)
        self.assertTrue(lines[-1].startswith("every pin is already in the base image: drop the '# base-pins' block"))

    def test_no_pins_is_fine(self):
        with open(self.path, "w") as f:
            f.write(f"FROM {BASE}\nUSER 101\n")
        self.assertEqual(c.check(self.path, fake_docker({}))[0], 0)

    def test_an_unpinned_base_is_an_error(self):
        with open(self.path, "w") as f:
            f.write("FROM nginx:latest\nRUN apk add --no-cache 'musl>=1.2.6-r3'   # base-pins\n")
        self.assertEqual(c.check(self.path, fake_docker({}))[0], 2)

    def test_the_real_web_dockerfile_has_its_pins_marked(self):
        here = os.path.dirname(os.path.abspath(__file__))
        base, pins = c.parse(os.path.join(here, "..", "web", "Dockerfile"))
        self.assertIn("@sha256:", base)
        self.assertEqual(set(pins), {"musl", "musl-utils", "zlib"})


if __name__ == "__main__":
    unittest.main()
