"""Portable Manila quota SQL semantics tests; MariaDB is tested separately."""
from pathlib import Path
import re
import sqlite3
import unittest

ROOT = Path(__file__).resolve().parents[1]
RESOURCES = "shares gigabytes snapshots snapshot_gigabytes share_networks share_replicas replica_gigabytes share_groups share_group_snapshots backups backup_gigabytes".split()
TYPED = "shares gigabytes snapshots snapshot_gigabytes share_replicas replica_gigabytes".split()
QUERIES = ("QuotaGetDefaultLimits", "QuotaGetProjectLimits", "QuotaGetUserLimits",
           "QuotaGetShareTypeLimits", "QuotaUsageGetProjectTotals", "QuotaUsageGetShareTypeTotals", "QuotaUsageGetAll")


def normalize(sql):
    return "\n".join(line.strip() for line in sql.splitlines()
                     if not line.lstrip().startswith("--")).strip().removesuffix(";")


class ManilaQuotaSQLTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.addCleanup(self.db.close)
        # Execute the checked-in MySQL fixtures, removing only MySQL DDL syntax.
        schema = (ROOT / "schema/manila/prereqs.sql").read_text() + (ROOT / "schema/manila/schema.sql").read_text()
        schema = re.sub(r"^\s*(?:UNIQUE )?KEY .*\n", "", schema, flags=re.M)
        schema = schema.replace("`id` int NOT NULL AUTO_INCREMENT PRIMARY KEY", "`id` INTEGER PRIMARY KEY")
        schema = re.sub(r",\s*\)", "\n)", schema)
        self.db.executescript(schema)
        source = (ROOT / "manila/db/queries.sql").read_text()
        generated = (ROOT / "manila/db/queries.sql.go").read_text()
        self.queries = {}
        for name in QUERIES:
            query = re.search(r"const " + name + r" = `(.*?)`", generated, re.S).group(1)
            original = source.split("-- name: " + name + " :many", 1)[1].split("-- name:", 1)[0]
            self.assertEqual(normalize(original), normalize(query), name)
            self.queries[name] = query

    def seed(self):
        self.db.executescript((ROOT / "tests/fixtures/manila_quotas.sql").read_text())

    def rows(self, name):
        return self.db.execute(self.queries[name]).fetchall()

    def test_integer_soft_delete_schema(self):
        self.seed()
        for table in ("quotas", "quota_usages", "quota_classes", "project_user_quotas", "project_share_type_quotas"):
            columns = {row[1]: row for row in self.db.execute(f"PRAGMA table_info({table})")}
            self.assertEqual("INTEGER", columns["deleted"][2])
            self.assertIsNone(columns["deleted"][4])
            self.assertGreater(self.db.execute(f"SELECT COUNT(*) FROM {table} WHERE deleted = id").fetchone()[0], 0)
            self.assertEqual([], self.db.execute(f"SELECT id FROM {table} WHERE typeof(deleted) <> 'integer' OR (deleted <> 0 AND deleted <> id)").fetchall())

    def test_raw_usage_preserves_users_and_share_types(self):
        self.seed()
        # Explicit fixture results, including duplicates: each user and share
        # type stays a separate row even though this API omits their IDs.
        expected = []
        for i, resource in enumerate(RESOURCES):
            expected.extend([("project-a", resource, i + 1, 1),
                             ("project-a", resource, 10, 2)])
        expected.extend(("project-a", resource, i + 4, 2)
                        for i, resource in enumerate(TYPED))
        expected.extend([
            ("project-a", "shares", 2, 4),  # Legacy NULL user.
            ("project-a", "shares", 3, 1),  # Two users under type-b.
            ("project-a", "shares", 5, 2),
            ("project-a", "shares", 10000, 1000),  # Empty share type.
            ("project-a", "shares", 9999, 9999),  # Active usage, deleted type.
            ("usage-only", "shares", 5, 2),
            ("usage-only", "gigabytes", 2000000000, 0),
            ("usage-only", "gigabytes", 2000000000, 0),
            ("type-usage-only", "snapshots", 1, 2),
            ("null-usage", "shares", None, None),
        ])
        self.assertCountEqual(expected, self.rows("QuotaUsageGetAll"))

    def test_project_limits_exclude_invalid_projects_and_deletions(self):
        self.seed()
        self.assertCountEqual([
            ("project-a", "shares", 20),
            ("project-a", "gigabytes", None),
            ("project-a", "snapshots", -1),
            ("project-a", "snapshot_gigabytes", 23),
            ("project-a", "share_networks", 0),
            ("project-a", "share_replicas", 25),
            ("project-a", "replica_gigabytes", 26),
            ("project-a", "share_groups", 27),
            ("project-a", "share_group_snapshots", 28),
            ("project-a", "backups", 29),
            ("project-a", "backup_gigabytes", 30),
            ("project-a", "per_share_gigabytes", 31),
        ], self.rows("QuotaGetProjectLimits"))

    def test_empty(self):
        for name in QUERIES:
            self.assertEqual([], self.rows(name), name)

    def test_project_totals_sum_users_exclude_share_types_and_deletions(self):
        self.seed()
        got = {(p, r): (used, reserved) for p, r, used, reserved in self.rows("QuotaUsageGetProjectTotals")}
        expected = {("project-a", r): (i + 11, 3) for i, r in enumerate(RESOURCES)}
        expected["project-a", "shares"] = (13, 7)  # Includes the legacy NULL user.
        expected.update({("usage-only", "shares"): (5, 2),
                         ("usage-only", "gigabytes"): (4000000000, 0),
                         ("null-usage", "shares"): (0, 0)})
        self.assertEqual(expected, got)

    def test_share_type_totals_never_add_project_usage(self):
        self.seed()
        got = {(p, tid, name, r): (used, reserved) for p, tid, name, r, used, reserved
               in self.rows("QuotaUsageGetShareTypeTotals")}
        expected = {("project-a", "type-a", "gold", r): (i + 4, 2) for i, r in enumerate(TYPED)}
        expected["project-a", "type-b", "silver", "shares"] = (8, 3)
        expected["type-usage-only", "type-a", "gold", "snapshots"] = (1, 2)
        self.assertEqual(expected, got)

    def test_limits_keep_scopes_and_preserve_null(self):
        self.seed()
        defaults = dict(self.rows("QuotaGetDefaultLimits"))
        self.assertEqual({"shares": 99, "snapshot_gigabytes": 777, "backups": None}, defaults)
        projects = {(p, r): n for p, r, n in self.rows("QuotaGetProjectLimits")}
        self.assertEqual(12, len(projects))
        self.assertEqual(20, projects["project-a", "shares"])
        self.assertIsNone(projects["project-a", "gigabytes"])
        self.assertEqual(-1, projects["project-a", "snapshots"])
        self.assertEqual(0, projects["project-a", "share_networks"])
        self.assertEqual([("project-a", "user-a", "shares", 1),
                          ("project-a", "user-b", "shares", 2),
                          ("user-only", "user-c", "shares", 3)], self.rows("QuotaGetUserLimits"))
        types = {(p, tid, r): n for p, tid, _, r, n in self.rows("QuotaGetShareTypeLimits")}
        self.assertEqual({("project-a", "type-a", "shares"): 7,
                          ("project-a", "type-a", "gigabytes"): None,
                          ("project-a", "type-b", "shares"): 0,
                          ("type-limit-only", "type-b", "snapshots"): -1}, types)

    def test_failure_is_an_error(self):
        self.db.execute("DROP TABLE quota_usages")
        for query in ("QuotaUsageGetProjectTotals", "QuotaUsageGetShareTypeTotals"):
            with self.assertRaises(sqlite3.OperationalError):
                self.rows(query)


if __name__ == "__main__":
    unittest.main()
