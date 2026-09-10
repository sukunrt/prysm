---
name: test
description: Run Prysm unit tests with go test for affected packages or specified tests.
---

# Unit test runner

Prefer `go test` over Bazel for unit tests. Run commands from the repository root.

## Usage
- `/test` — test affected packages from the working-copy diff
- `/test ./beacon-chain/sync` — test a specific package
- `/test ./validator/client -run 'TestRolesAt'` — run selected tests
- `/test ./...` — test everything (slow)

## Steps

1. **Determine packages** — use the packages or tests the user specified;
   otherwise identify changed Go files with `jj diff --name-only` (or
   `git diff --name-only HEAD` in a Git checkout) and test their containing
   packages. Use Go package paths such as `./validator/client`.

2. **Run**:
   ```bash
   go test -mod=readonly -count=1 <packages>
   ```
   Add `-run '<pattern>'` for focused checks and package-required build tags
   when applicable. `-mod=readonly` prevents tests from editing module files.

3. **Report**:
   ```
   ✅ Passed: X
   ❌ Failed: Y
     - package / test name — error summary
   ```
   Include the command and any failures or checks that could not run. If a
   failure passes on retry, report the flakiness rather than hiding the first result.
