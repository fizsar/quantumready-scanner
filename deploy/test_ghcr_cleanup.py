"""Tests of the GHCR cleanup plan (no network): python3 deploy/test_ghcr_cleanup.py"""
import datetime as dt
import os
import sys
import unittest

sys.dont_write_bytecode = True  # no deploy/__pycache__ in the working tree
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from ghcr_cleanup import PlanError, plan, read_deployments  # noqa: E402

NOW = dt.datetime(2026, 11, 30, 12, 0, tzinfo=dt.timezone.utc)


def sha(n):
    return f"{n:07x}" + "0" * 33


def versions(*tags):
    return [{"id": i, "tags": list(t) if isinstance(t, tuple) else [t]} for i, t in enumerate(tags)]


def kept_tags(vs, keep):
    return sorted(t for v in vs if v["id"] in keep for t in v["tags"])


class ReadDeployments(unittest.TestCase):
    def test_lines(self):
        got = read_deployments("# comment\n\n2026-10-15 sha-2f41b0b\n2026-10-29T23:15Z sha-62eac88\n")
        self.assertEqual([t for _, t in got], ["sha-2f41b0b", "sha-62eac88"])
        self.assertEqual(got[1][0], dt.datetime(2026, 10, 29, 23, 15, tzinfo=dt.timezone.utc))

    def test_bad_line(self):
        for bad in ["2026-10-15 latest", "sha-2f41b0b", "2026-10-15  sha-2f41b0b extra"]:
            with self.assertRaises(PlanError, msg=bad):
                read_deployments(bad)

    def test_the_repository_file(self):
        here = os.path.dirname(os.path.abspath(__file__))
        with open(os.path.join(here, "deployments.txt"), encoding="utf-8") as f:
            got = read_deployments(f.read())
        self.assertGreaterEqual(len(got), 2)
        self.assertEqual([w for w, _ in got], sorted(w for w, _ in got), "oldest first")


class Plan(unittest.TestCase):
    def deps(self, *lines):
        return read_deployments("\n".join(lines))

    def test_production_rollback_recent_branches(self):
        vs = versions("sha-0000001", "sha-0000002", "sha-0000003", "sha-0000004", "sha-0000005",
                      "sha-0000006", ("sha-0000007", "latest"), "sha-0000008", "sha-0000009")
        deps = self.deps("2026-09-01 sha-0000001",   # old, not production: goes
                         "2026-10-20 sha-0000002",   # 41 days ago: goes... but it is the rollback
                         "2026-11-25 sha-0000003")   # production
        branches = {"main": [sha(9), sha(7), sha(99)], "showcase": [sha(8), sha(6)]}
        keep, delete = plan(vs, deps, branches, NOW, days=30, per_branch=2)
        self.assertEqual(kept_tags(vs, keep), ["latest", "sha-0000002", "sha-0000003", "sha-0000006",
                                               "sha-0000007", "sha-0000008", "sha-0000009"])
        self.assertEqual(sorted(t for v in delete for t in v["tags"]), ["sha-0000001", "sha-0000004", "sha-0000005"])
        why = {t: keep[v["id"]] for v in vs for t in v["tags"] if v["id"] in keep}
        self.assertIn("production", why["sha-0000003"])
        self.assertIn("rollback", why["sha-0000002"])
        self.assertEqual(why["sha-0000009"], ["main #1"])
        self.assertEqual(why["sha-0000006"], ["showcase #2"])

    def test_deployed_in_the_window(self):
        vs = versions("sha-0000001", "sha-0000002", "sha-0000003", "sha-0000004")
        deps = self.deps("2026-10-30T11:59Z sha-0000001",  # 31 days ago: out
                         "2026-10-31T12:00Z sha-0000002",  # 30 days ago: in
                         "2026-11-20 sha-0000003",
                         "2026-11-21 sha-0000004")
        keep, delete = plan(vs, deps, {}, NOW, days=30)
        self.assertEqual(kept_tags(vs, keep), ["sha-0000002", "sha-0000003", "sha-0000004"])
        self.assertEqual([v["tags"] for v in delete], [["sha-0000001"]])

    def test_per_branch_counts_builds_not_commits(self):
        vs = versions("sha-0000003", "sha-0000005", "sha-0000009")
        deps = self.deps("2026-11-29 sha-0000009")
        keep, _ = plan(vs, deps, {"main": [sha(n) for n in range(10, 0, -1)]}, NOW, per_branch=2)
        self.assertEqual(kept_tags(vs, keep), ["sha-0000005", "sha-0000009"])

    def test_untagged_extra_and_shared_tags_kept(self):
        vs = versions((), "sha-0000001", ("sha-0000002", "sha-00000ff"), "sha-0000003")
        deps = self.deps("2026-11-29 sha-00000ff")
        keep, delete = plan(vs, deps, {}, NOW, extra=["sha-0000001", "sha-0000bad"])
        self.assertEqual(kept_tags(vs, keep), ["sha-0000001", "sha-0000002", "sha-00000ff"])
        self.assertIn(0, keep)
        self.assertEqual([v["tags"] for v in delete], [["sha-0000003"]])

    def test_refuses_a_wrong_file(self):
        vs = versions("sha-0000001", "sha-0000002")
        with self.assertRaises(PlanError):
            plan(vs, [], {}, NOW)
        with self.assertRaisesRegex(PlanError, "production"):
            plan(vs, self.deps("2026-11-01 sha-0000001", "2026-11-02 sha-0000abc"), {}, NOW)
        with self.assertRaisesRegex(PlanError, "rollback"):
            plan(vs, self.deps("2026-01-01 sha-0000abc", "2026-11-02 sha-0000001"), {}, NOW)

    def test_nothing_kept_means_nothing_deleted_by_mistake(self):
        # Every version is either kept or deleted, never both.
        vs = versions(*[f"sha-{n:07x}" for n in range(1, 30)])
        deps = self.deps("2026-11-29 sha-0000001")
        keep, delete = plan(vs, deps, {"main": [sha(n) for n in range(29, 0, -1)]}, NOW, per_branch=5)
        self.assertEqual(len(keep) + len(delete), len(vs))
        self.assertFalse({v["id"] for v in delete} & set(keep))


if __name__ == "__main__":
    unittest.main(verbosity=1)
