"""Portable SQL semantics tests; native MariaDB integration is tested separately.
Run: python3 -m unittest discover -s tests -p octavia_quota_sql_test.py -v
No external databases, credentials, or third-party Python dependencies.
"""
from pathlib import Path
import re
import sqlite3
import unittest

ROOT = Path(__file__).resolve().parents[1]
RESOURCES = ("load_balancer", "listener", "pool", "member", "health_monitor", "l7policy", "l7rule")

class QuotaUsageSQLTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.addCleanup(self.db.close)
        self.db.executescript((ROOT / "schema/octavia/schema.sql").read_text())
        source = (ROOT / "octavia/db/repositories/queries.sql").read_text().split("-- name: QuotaUsageGetAll :many", 1)[1]
        generated = (ROOT / "octavia/db/repositories/queries.sql.go").read_text()
        self.query = re.search(r"const QuotaUsageGetAll = `(.*?)`", generated, re.S).group(1)
        def sql_without_comments(text):
            return "\n".join(line for line in text.splitlines() if not line.lstrip().startswith("--")).strip().removesuffix(";")
        self.assertEqual(sql_without_comments(source), sql_without_comments(self.query))

    def test_empty(self):
        self.assertEqual([], self.db.execute(self.query).fetchall())

    def test_all_resources_statuses_projects_and_shared_parents(self):
        self.db.executescript((ROOT / "tests/fixtures/octavia_quota_usage.sql").read_text())
        rows = self.db.execute(self.query).fetchall()
        self.assertEqual(14, len(rows))
        expected = {(project, resource): count for index, resource in enumerate(RESOURCES)
                    for project, count in (("project-a", index + 4), ("project-b", 1))}
        self.assertEqual(expected, {(p, r): n for p, r, n in rows})
        # project-b has no quota override: usage must still be counted.
        self.assertEqual([], self.db.execute("SELECT * FROM quotas WHERE project_id='project-b'").fetchall())
        # Cached usage is deliberately wrong and must not supply object counts.
        self.assertEqual(999, self.db.execute("SELECT in_use_member FROM quotas").fetchone()[0])

    def test_legacy_project_counts_exclude_missing_ids(self):
        self.db.executescript((ROOT / "tests/fixtures/octavia_quota_usage.sql").read_text())
        source = (ROOT / "octavia/db/repositories/queries.sql").read_text()
        generated = (ROOT / "octavia/db/repositories/queries.sql.go").read_text()
        for name, count in (("LoadBalancerCountByProject", 4),
                            ("ListenerCountByProject", 5),
                            ("PoolCountByProject", 6)):
            with self.subTest(query=name):
                query = re.search(r"const " + name + r" = `(.*?)`", generated, re.S).group(1)
                source_query = source.split("-- name: " + name + " :many", 1)[1].split("-- name:", 1)[0]
                self.assertEqual(source_query.strip().removesuffix(";"),
                                 query.split("\n", 1)[1].strip())
                self.assertEqual({"project-a": count, "project-b": 1},
                                 dict(self.db.execute(query).fetchall()))

    def test_failed_query_is_an_error(self):
        self.db.execute("DROP TABLE member")
        with self.assertRaises(sqlite3.OperationalError):
            self.db.execute(self.query).fetchall()

    def test_nullable_limits_and_unfiltered_quota_projects(self):
        self.db.executescript((ROOT / "tests/fixtures/octavia_quota_usage.sql").read_text())
        self.db.execute("INSERT INTO quotas (project_id) VALUES ('')")
        generated = (ROOT / "octavia/db/repositories/queries.sql.go").read_text()
        query = re.search(r"const QuotaGetAll = `(.*?)`", generated, re.S).group(1)
        rows = {row[0]: row[1:] for row in self.db.execute(query).fetchall()}
        self.assertEqual({"project-a": (None, -1, 0, 0, 17, None, 23),
                          "": (None,) * 7}, rows)

    def test_existing_repositories_keep_non_deleted_unowned_objects(self):
        self.db.executescript((ROOT / "tests/fixtures/octavia_quota_usage.sql").read_text())
        generated = (ROOT / "octavia/db/repositories/queries.sql.go").read_text()
        for name in ("PoolGetAll", "LoadBalancerGetAllWithVIP"):
            with self.subTest(query=name):
                query = re.search(r"const " + name + r" = `(.*?)`", generated, re.S).group(1)
                rows = self.db.execute(query).fetchall()
                self.assertTrue(any(row[1] is None for row in rows))
                self.assertTrue(any(row[1] == "" for row in rows))
                self.assertTrue(any("ERROR" in row for row in rows))
                self.assertFalse(any("DELETED" in row for row in rows))

if __name__ == "__main__":
    unittest.main()
