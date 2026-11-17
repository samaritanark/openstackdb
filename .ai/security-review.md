# Security review

Focused security review of the changes on the current branch of `openstackdb`. Read `/AGENTS.md` first for project context.

## Modes

- **Change review (default):** only what this branch introduces, compared with a base ref.
- **Baseline review:** the whole repository. Use it when the user asks for a full or baseline review, or when there is no diff to review. In step 1, list every hand-written runtime file and every `queries.sql` instead of diffing. In step 2, "introduced by this change set" means "present in the repository".

Before judging anything, read `docs/explanation/security-model.md` and `docs/reference/security-allowances.md`. A candidate that matches an allowance (AL), privilege (PR), external control (EX) or residual risk (R) entry is known and is not reported. A candidate that contradicts an entry is a defect, even if the code looks deliberate.

## 1. Gather the change set

The default base is `origin/HEAD`. Use another ref if the user names one.

```sh
git status --short --branch
git diff --name-only --merge-base origin/HEAD   # committed + uncommitted changes
git log --no-decorate --oneline origin/HEAD..HEAD
git diff --merge-base origin/HEAD
```

Untracked files (`??` in the status output) are not in the diff. Read any untracked `.go` or `.sql` files and treat them as new code.

## 2. Objective

Find HIGH-CONFIDENCE vulnerabilities **introduced by this change set** that have real exploitation potential. This is not a general code review. Do not report pre-existing issues, style, or hardening wishes.

## 3. Threat model

`openstackdb` is a read-only Go library. Callers are trusted to own credentials, pools, and authorization, but **filter values** (project IDs, UUIDs, names, states, time windows) often come from end users of the calling service.

**SQL injection (highest priority)**
- `queries.sql` → `queries.sql.go` is sqlc-generated and parameterized. Review the `.sql` source. A generated file only matters if it was hand-edited or no longer matches its source.
- Hand-built SQL lives in `<pkg>/filters.go` and `internal/filter`. `filter.Builder` takes **trusted column names** and appends values to `Args`. Flag any change that:
  - passes a caller-controlled string as a column, table, `ORDER BY`, sort direction, or `LIMIT`/`OFFSET` fragment;
  - builds SQL with `fmt.Sprintf`, `+`, or `strings.Join` over values instead of `?` placeholders;
  - lets the number of placeholders drift from `len(Args)`, so parameters bind to the wrong positions.
- Any `Exec`/`Query`/`QueryContext` with a non-constant SQL string built from outside the package.

**Fail-open filtering (cross-tenant data exposure)**
Callers scope results with filters such as `ProjectIDs`. The contract: a `nil` slice is unrestricted, a non-nil empty slice matches nothing (`1 = 0`), and the zero `Deleted` value is `ExcludeDeleted`. Flag any change that lets an empty or invalid filter widen results:
- the `1 = 0` branch is dropped, or a filter field is ignored;
- an unknown `Deleted` value is treated as include;
- a lifetime window silently becomes unbounded;
- an `OR` escapes the AND-joined clauses because of missing parentheses;
- archive or shadow-table modes return rows the caller did not ask for.

**Read-only guarantee**
Flag new `INSERT`/`UPDATE`/`DELETE`/DDL, `SELECT … FOR UPDATE`, stored-procedure calls, or `multiStatements` enabled in non-test code.

**DSN and connection handling (`dsn.go`, `db.go`)**
- Credentials leaking into error strings, `String()`/`fmt` output, or panics (e.g. wrapping the raw URL or DSN in an error).
- DSN construction that lets a username, password, host, or database name containing `@`, `/`, `?`, `(` or `)` reroute the connection or inject driver parameters.
- Dangerous go-sql-driver parameters enabled by default or from untrusted input: `allowAllFiles` (lets the server read local files via LOAD DATA LOCAL INFILE), `allowCleartextPasswords`, `allowOldPasswords`, `tls=skip-verify`, `tls=false`, `multiStatements`.

**Sensitive column exposure**
New queries or model fields that start returning secrets, such as:
- Keystone `password` or `credential` blobs;
- Nova admin passwords in `instance_system_metadata`, or `user_data`;
- Heat `raw_template` or parameters;
- Ironic `driver_info` (BMC credentials);
- Magnum or Octavia certificates and private keys;
- Manila access keys.

Report these only when the column is newly selected and not clearly required by the function's documented purpose.

**Out of scope**
- `tests/`, `*_test.go`, and `internal/testutil`, including fixture credentials such as `test:test`.
- `schema/*.sql`, which are test fixtures.
- Markdown docs and dependency versions.

## 4. Method

1. **Context.** Before judging the diff, read the existing safe patterns: `internal/filter/filter.go`, the touched package's `filters.go` and `queries.sql`, `dsn.go`, and the filter-semantics section of `README.md`. Look for deviations from them.
2. **Find candidates.** For each changed file, trace data from exported function parameters to SQL text, query arguments, errors, and returned fields. For each candidate, record the file, line, category, data flow, and exploit scenario.
3. **Verify.** Check each candidate independently against `.ai/security-review-false-positives.md` and give it a 1–10 confidence score. If your tool supports subagents, run one verifier per candidate in parallel and give each the candidate plus that file. Verifiers only read code; they do not run commands or edit files.
4. **Filter.** Drop findings scoring below 8. Keep only HIGH and MEDIUM severity.

If the diff touches none of the areas above and nothing else is security-relevant, report no findings without running the full process.

## 5. Output

Reply with a markdown report and nothing else. For each finding:

```markdown
# Vuln 1: SQL injection: `nova/db/main/filters.go:42`

* Severity: High
* Confidence: 9/10
* Category: sql_injection
* Description: `InstanceFilters.SortKey` is concatenated into the `ORDER BY` clause without an allowlist.
* Exploit scenario: A caller forwards a user's `sort` query parameter. The value `uuid, (SELECT SLEEP(5))` runs attacker-chosen subqueries under the library's DB account, and boolean subqueries can read other tenants' rows.
* Recommendation: Map the caller's sort key through a fixed `map[string]string` of allowed columns and reject unknown keys.
```

Severity:
- **High**: directly exploitable, e.g. SQL injection, cross-tenant data exposure, credential disclosure, or arbitrary file read through the driver.
- **Medium**: significant impact, but only with specific caller behavior or configuration.
- **Low**: do not report.

If no findings remain, reply `No high-confidence security findings in this diff.` followed by one line listing the areas you checked.

## 6. Record the review

Do this when the user asks to record the review, and always for a baseline review:

1. **Full report:** write it to `local/security-review/<YYYY-MM-DD>/report.md`. Include the findings, the candidates you rejected and why, and the areas checked. `local/` is gitignored. Reports stay there until their fixes ship, because they may contain working exploit detail.
2. **Findings:** give each finding the next `SR-<n>` ID and add it to "Open findings" in `docs/reference/security-allowances.md`. Write one sanitized line per finding, without a working exploit.
3. **Accepted risks:** add each new accepted risk or deliberate behavior to the register as AL, EX or R, and only with the user's agreement.
4. **History:** add a row to "Review history" in `docs/explanation/security-model.md`.
5. **Fixed findings:** when a fix ships, mark its SR entry fixed, with the commit, and update the security model's controls table and tests column.
