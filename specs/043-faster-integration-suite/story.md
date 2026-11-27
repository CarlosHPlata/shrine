# Faster integration suite

## Story
As a shrine maintainer, I want the integration suite to run much faster, so that I can run it locally without tying up the machine and CI stays well inside its timeout.

## Acceptance criteria
- [ ] Given the full integration suite, when it runs on the same machine as the baseline below, then its wall time is clearly lower (target: at least half).
- [ ] Given any behavior the suite checks today, when the work is done, then it is still checked, either by an integration test or by a unit test that replaced it.
- [ ] Given the suite run several times in a row, when tests share setup or run in parallel, then results are the same on every run (no new flakiness).
- [ ] Given any test touched by this work, when it is read, then its body is still a short list of recipe-style steps and assertions; shared setup, cleanup, locking and parallelism live in `tests/integration/testutils`, not in the test files.

## Baseline (measured 2026-11-26)
- Full suite: **657s** (about 11 minutes) on the dev machine (WSL2, Docker). About 250 subtests across 22 files, all run one after another.
- CI runs the same suite through `make test-integration`, which sets `-timeout 5m`. CI has not been timed; if a runner were as slow as the dev machine, the run would time out.
- `TestGetDocker`: 10 subtests, 43s, about 4s each. Most of them only run a read-only `shrine get`; nearly all of the time is setup and teardown.
- No per-test timings exist for the rest of the suite yet. Only one full run and the `TestGetDocker` numbers were measured.

## Where the time goes
- Every Docker subtest starts from scratch. `NewDockerSuite` (`tests/integration/testutils/dockersuite.go`) cleans up the team's Docker resources in `BeforeEach` and again in `AfterEach`, and most suites then apply teams and deploy in their own `BeforeEach`.
- `Suite` (`tests/integration/testutils/testsuite.go`) only has `BeforeEach`/`AfterEach`. There is no hook that runs once per suite, so read-only tests cannot share a deployment.
- Nothing calls `t.Parallel()`.

## Plan
Do step 0 first, then the options in order, measuring after each one. Stop when the target is met.

0. **Measure.** Run the suite once with `-json` and rank tests by elapsed time. Use that list to pick where to apply the steps below. This is one more full-suite run.
1. **Deploy once per group of read-only tests.** Add a once-per-suite setup and teardown hook to `Suite`. Move tests that only read state (`get`, `describe`, `status`, JSON listings) onto one shared deployment. Tests that change state keep their per-test setup. Expected for `TestGetDocker`: about 43s down to under 10s.
2. **Move tests that don't need Docker down to unit tests.** Candidates: validation rejections, output formatting, JSON shape. Check them against the handlers with a fake backend or seeded state. Start with `traefik_plugin_test.go` (44 subtests).
3. **Run top-level tests in parallel.** Requirements before any `t.Parallel()`:
   - each parallel suite uses its own team name, because cleanup is keyed on the team and would otherwise remove another test's containers;
   - subnet allocation (`SeedSubnetState`) is safe when two tests start at the same time;
   - tests that bind fixed host ports or share the Traefik setup stay serial.
   This has the largest possible gain and the highest risk of flakiness.
4. **Split the suite across CI jobs.** A matrix in `.github/workflows/ci.yml` that runs a few `-run` groups in parallel. Cuts CI wall time only; no help locally.
5. **Cheaper teardown.** Check how `cleanupDockerResources` stops containers. If it waits for a graceful stop, force-remove them or use a zero stop timeout.

## Related findings
- **Flaky test, separate fix.** `TestGetDocker/get deployed --json matches the table row for row` fails about half the time. `DeploymentStore.List` (`internal/state/local/deployments.go:74`) ranges over a Go map, so each CLI call lists deployments in a random order, and the test compares the order of two separate calls. Fix: sort by team and name in `List` or in `collectAllDeployments` (`internal/handler/deployments.go`). This changes user-visible output order, so it needs its own change. It should land before step 1, because shared deployments make ordering assumptions more common.
- **CI timeout.** Once CI timings are known, set `-timeout` in the Makefile to match the real run time with some headroom.
- **Overlap with 042.** `specs/042-recipe-style-integration-tests` rewrites the same test files onto the recipe-style helpers. Do one before the other rather than both at once; 042 first makes the setup blocks shorter and easier to move into shared hooks.

## Out of scope
- Dropping coverage to save time.
- Changes to production behavior, other than the ordering fix listed above as its own change.
- Replacing Docker with a fake in the tests that exist to check real containers.

## Verification
- Compare full-suite wall time before and after on the same machine.
- Run the suite at least three times in a row after steps 1 and 3 to check for flakiness.
- Project rule: the full suite is only run when explicitly agreed; day-to-day work runs single tests with `-run`.
