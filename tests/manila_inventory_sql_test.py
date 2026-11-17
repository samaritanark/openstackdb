"""Logical-share inventory: execute authoritative and generated SELECTs."""
import json
from pathlib import Path
import re
import sqlite3
import unittest

ROOT = Path(__file__).resolve().parents[1]


def normalize(sql):
    return '\n'.join(line.strip() for line in sql.splitlines()
                     if not line.lstrip().startswith('--')).strip().removesuffix(';')


class ManilaInventorySQLTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(':memory:')
        self.addCleanup(self.db.close)
        schema = (ROOT / 'schema/manila/prereqs.sql').read_text() + (ROOT / 'schema/manila/schema.sql').read_text()
        schema = re.sub(r'^\s*(?:UNIQUE )?KEY .*\n', '', schema, flags=re.M)
        schema = schema.replace('`id` int NOT NULL AUTO_INCREMENT PRIMARY KEY', '`id` INTEGER PRIMARY KEY')
        schema = re.sub(r',\s*\)', '\n)', schema)
        self.db.executescript(schema)
        source = (ROOT / 'manila/db/queries.sql').read_text()
        generated = (ROOT / 'manila/db/queries.sql.go').read_text()
        self.queries = {}
        for name in ('ShareGetInventory', 'ShareGetAllWithInstances'):
            query = re.search(r'const ' + name + r' = `(.*?)`', generated, re.S).group(1)
            original = source.split('-- name: ' + name + ' :many', 1)[1].split('-- name:', 1)[0]
            self.assertEqual(normalize(original), normalize(query))
            self.queries[name] = query

    def test_empty(self):
        self.assertEqual([], self.db.execute(self.queries['ShareGetInventory']).fetchall())

    def test_inventory(self):
        self.db.executescript((ROOT / 'tests/fixtures/manila_inventory.sql').read_text())
        expected = json.loads((ROOT / 'tests/fixtures/manila_inventory_expected.json').read_text())
        rows = self.db.execute(self.queries['ShareGetInventory']).fetchall()
        self.assertEqual(len(expected), len(rows))
        self.assertEqual(expected, {row[0]: list(row) for row in rows})
        self.assertEqual(sorted(expected), [row[0] for row in rows])
        self.assertEqual(10 * len(expected), sum(row[3] for row in rows))
        legacy = self.db.execute(self.queries['ShareGetAllWithInstances']).fetchall()
        self.assertEqual(32, len(legacy))  # Intentional reusable instance API.
        self.assertEqual(2, sum(row[0] == 'identical' for row in legacy))
        self.assertEqual(1, sum(row[0] == 'identical' for row in rows))

    def test_insertion_order_does_not_choose_metadata(self):
        fixture = (ROOT / 'tests/fixtures/manila_inventory.sql').read_text().splitlines()
        inserts = [line for line in fixture if line.startswith('INSERT INTO share_instances')]
        others = [line for line in fixture if not line.startswith('INSERT INTO share_instances')]
        self.db.executescript('\n'.join(others + inserts[::-1]))
        expected = json.loads((ROOT / 'tests/fixtures/manila_inventory_expected.json').read_text())
        self.assertEqual(expected, {row[0]: list(row) for row in self.db.execute(self.queries['ShareGetInventory'])})


    def test_upstream_status_and_replica_precedence(self):
        # Independently express Share.instance's sort/filter algorithm. Input
        # relationship order is ID order, our explicit tie break where upstream
        # does not specify one. Cover every transitional status, including the
        # entries already promoted into the preferred-status prefix.
        preferred = ['reverting', 'replication_change', 'migrating',
                     'server_migrating', 'available', 'error']
        transitional = ['creating', 'deleting', 'manage_starting',
                        'unmanage_starting', 'extending', 'shrinking',
                        'migrating', 'migrating_to', 'restoring', 'reverting',
                        'server_migrating', 'server_migrating_to',
                        'backup_restoring', 'backup_creating', 'ensuring']
        statuses = list(dict.fromkeys(preferred + transitional +
                                     ['inactive', 'future_state', None]))
        self.db.execute("INSERT INTO shares(id, size, deleted) VALUES('s',10,'False')")
        for left in statuses:
            for right in statuses:
                for replicas in [(None, None), ('active', 'in_sync'),
                                 ('in_sync', 'active'), ('active', 'active')]:
                    with self.subTest(statuses=(left, right), replicas=replicas):
                        pair = [left, right]
                        order = preferred + [x for x in pair if x not in preferred
                                             and x not in transitional] + transitional
                        ranked = sorted(range(2), key=lambda i: order.index(pair[i]))
                        if pair[ranked[0]] != 'replication_change':
                            ranked = [i for i in ranked if replicas[i] == 'active'] or ranked
                        winner = ranked[0]
                        self.db.execute('DELETE FROM share_instances')
                        self.db.executemany("INSERT INTO share_instances(id,share_id,status,replica_state,share_type_id,deleted,cast_rules_to_readonly) VALUES(?, 's', ?, ?, ?, 'False', 0)",
                                            [(str(i), pair[i], replicas[i], str(i)) for i in range(2)])
                        row = self.db.execute(self.queries['ShareGetInventory']).fetchone()
                        self.assertEqual(pair[winner], row[5])
                        self.assertEqual(str(winner), row[6])

if __name__ == '__main__':
    unittest.main()
