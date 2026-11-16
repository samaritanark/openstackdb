"""Portable tests of the actual generated Cinder SQL; no database service."""
from pathlib import Path
import re
import sqlite3
import unittest

ROOT = Path(__file__).resolve().parents[1]
QUERIES = ("QuotaGetActiveLimits", "QuotaGetActiveUsage",
           "QuotaGetDefaultClassLimits", "VolumeTypeGetAllCurrent")

class CinderQuotaSQLTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.addCleanup(self.db.close)
        schema = (ROOT / "schema/cinder/schema.sql").read_text()
        schema = schema.replace("`id` int NOT NULL AUTO_INCREMENT PRIMARY KEY", "`id` INTEGER PRIMARY KEY")
        self.db.executescript(schema)
        self.db.executescript((ROOT / "tests/fixtures/cinder_quotas.sql").read_text())
        source = (ROOT / "cinder/db/queries.sql").read_text()
        generated = (ROOT / "cinder/db/queries.sql.go").read_text()
        self.queries = {}
        def normalize(sql):
            return " ".join(line.strip() for line in sql.splitlines()
                            if not line.lstrip().startswith("--")).strip().removesuffix(";")
        for name in QUERIES:
            sql = re.search(r"const " + name + r" = `(.*?)`", generated, re.S).group(1)
            original = source.split("-- name: " + name + " :many", 1)[1].split("-- name:", 1)[0]
            self.assertEqual(normalize(sql), normalize(original))
            self.queries[name] = sql

    def rows(self, name):
        return self.db.execute(self.queries[name]).fetchall()

    def test_active_overrides_preserve_zero_unlimited_and_null(self):
        rows = self.rows("QuotaGetActiveLimits")
        self.assertEqual(len(rows), 5)
        self.assertIn(("project-a", "volumes", 0), rows)
        self.assertIn(("project-a", "snapshots", -1), rows)
        self.assertIn(("bad-null-limit", "volumes", None), rows)
        self.assertIn((None, "volumes", 5), rows)
        self.assertFalse(any(r[-1] == 999 for r in rows))

    def test_usage_only_and_typed_accounting_stay_separate(self):
        rows = self.rows("QuotaGetActiveUsage")
        self.assertEqual(len(rows), 4)
        self.assertIn(("usage-only", "backups", 7, 4), rows)
        self.assertIn(("project-a", "gigabytes", 100, 15), rows)
        self.assertIn(("project-a", "gigabytes_fast_ssd", 80, 10), rows)

    def test_only_active_default_class_and_types(self):
        self.assertEqual(set(self.rows("QuotaGetDefaultClassLimits")),
                         {("volumes", 20), ("snapshots_fast_ssd", 8)})
        self.assertEqual(set(self.rows("VolumeTypeGetAllCurrent")),
                         {("type-1", "fast_ssd"), ("type-2", "__DEFAULT__")})

    def test_duplicates_are_not_silently_summed(self):
        self.db.execute("INSERT INTO quota_usages (project_id, resource, in_use, reserved, deleted) VALUES ('project-a', 'volumes', 8, 9, 0)")
        rows = [r for r in self.rows("QuotaGetActiveUsage") if r[0:2] == ("project-a", "volumes")]
        self.assertEqual(len(rows), 2)  # Caller must flag ambiguity.

    def test_duplicate_limits_and_defaults_are_preserved(self):
        self.db.execute("INSERT INTO quotas (project_id, resource, hard_limit, deleted) VALUES ('project-a', 'volumes', 9, 0)")
        self.db.execute("INSERT INTO quota_classes (class_name, resource, hard_limit, deleted) VALUES ('default', 'volumes', -1, 0)")
        limits = [r for r in self.rows("QuotaGetActiveLimits")
                  if r[:2] == ("project-a", "volumes")]
        defaults = [r for r in self.rows("QuotaGetDefaultClassLimits")
                    if r[0] == "volumes"]
        self.assertCountEqual(limits, [("project-a", "volumes", 0),
                                       ("project-a", "volumes", 9)])
        self.assertCountEqual(defaults, [("volumes", 20), ("volumes", -1)])

    def test_nullable_and_empty_keys_remain_visible(self):
        self.db.execute("INSERT INTO quota_usages (project_id, resource, in_use, reserved, deleted) VALUES (NULL, NULL, 0, -1, 0), ('', '', 1, 2, 0), ('excluded', 'volumes', 999, 999, NULL)")
        self.db.execute("INSERT INTO quota_classes (class_name, resource, hard_limit, deleted) VALUES ('default', NULL, NULL, 0), (NULL, 'ignored', 999, 0), ('default', 'ignored', 999, NULL)")
        usage = self.rows("QuotaGetActiveUsage")
        self.assertEqual(len(usage), 6)
        self.assertIn((None, None, 0, -1), usage)
        self.assertIn(("", "", 1, 2), usage)
        self.assertCountEqual(self.rows("QuotaGetDefaultClassLimits"),
                              [("volumes", 20), ("snapshots_fast_ssd", 8),
                               (None, None)])

    def test_signed_integer_boundaries_are_not_aggregated(self):
        self.db.execute("INSERT INTO quota_usages (project_id, resource, in_use, reserved, deleted) VALUES ('bounds', 'volumes', -2147483648, 2147483647, 0)")
        self.assertIn(("bounds", "volumes", -2147483648, 2147483647),
                      self.rows("QuotaGetActiveUsage"))

if __name__ == "__main__":
    unittest.main()
