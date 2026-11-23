# Review of Release 1 artifact 7720ffc

## Metadata

| Field | Value |
|---|---|
| Reviewer id | `grok-4.7-medium` (Grok 4.7 Medium, no Fast) |
| Reviewer | grok-4.7-medium |
| Author | Aleksandr Mazhul |
| SHA | `7720ffc36c90c4475ea3f273bfd6b90f066ebb9e` |
| Date | 2026-11-23 |
| Tree | `origin/main` at `7720ffc` (`test(e2e): assert the 360 px viewport`, parent `e61c1e6`) |
| Goal text | `briefs/RELEASE1-GOAL.md` on `origin/docs/r1-repo-handoff` (`b8c41ebba2771d3587cf843e21863e131c677334`). That file is absent from `7720ffc`. |
| Attestation pointer | `cursor/release1-attestation-0c1c` at `2ea10c92cc6dea6ff55856580858f59269b39a6b` (PR #69), not merged. CI conclusions below were read from GitHub Actions run [45195641485](https://github.com/knowgap-lab/ai-mentor/actions/runs/45195641485), head SHA `7720ffc`. |
| This session | Read the tree, workflow, `TestContractLiveE3`, Playwright sources, auth startup, and the Actions job list. Did not re-run Playwright, `TestContractLiveE3`, `catalog_ci`, or the live lesson HTTP script. |

## Requirements checklist

| Item | Status | Evidence |
|---|---|---|
| Release 1 scope only, synthetic data (`briefs/RELEASE1-GOAL.md`) | pass | Demo catalog `content/tests/fixtures/demo-catalog/ct-demo` is labeled synthetic (`package.yaml` title «Демонстрационный пакет (синтетический, D142)»). Seed names are `Демо Ученик`, `tg-demo-tutor`, `tg-demo-student` (`tools/dev_seed_scenario.sql`, `README.md`). `tools/release1_inventory.py` keeps `putRtImport` out of release 1. No optimizer or knowledge-model service under `core/`. The historical README still describes a later LLM idea; it is not a running release-1 feature. |
| `release1-ci` jobs except `deploy` | pass | `.github/workflows/release1-ci.yml` job `name:` values match `CI_JOBS` in `tools/release1_done.py`. Run 45195641485 on this SHA: `tools/check.sh`, `tools/check-code.sh`, `generation` (regenerate step then `git diff --exit-code`), `build`, `storybook`, `size-limit`, `e2e`, `contract-live`, `docker`, `chrome` — conclusion `success`. `docker` is a Postgres 16.15 readiness smoke; `chrome` prints `google-chrome-stable --version`. |
| `contract-live` runs `TestContractLiveE3` | pass | `tools/ci/contract-live.sh` runs `go test ./cmd/server/ -run '^TestContractLiveE3$'`. Job `contract-live` on run 45195641485 succeeded. The test uses `dbtest` Postgres and `rootHandler`: 401 without a session, tutor GET lesson 200, student correct answer with `verdict=correct`, repeat 200, integer answer 200, tutor attempts 200, foreign tutor 403. |
| Playwright e2e, including a 360 px assertion | pass | `tools/e2e/playwright.config.ts` and `VIEWPORT_MOBILE` set `{ width: 360, height: 800 }`. `tools/e2e/browser-coverage.spec.ts` test `no_horizontal_scroll — задача на 360 px` asserts `page.viewportSize()?.width` is 360, then checks the task page for horizontal scroll. Job `e2e` (`npm test` in `tools/e2e`) succeeded on this SHA. |
| No pupil PII in the public tree; dev-auth boundary holds for the mounted route | pass | No email addresses and no numeric `telegram_id` literals in `*.md`, `*.yml`, `*.yaml`, `*.json`, `*.sql`, `*.go`, `*.py`, `*.ts`, `*.tsx`. `POST /auth/dev` is mounted only when `devStackEnabled()` is true (`ENV=local`, `AUTH_DEV_MODE=true`, `DATABASE_URL` set) in `core/cmd/server/app.go`. `loginDev` returns 403 unless `Env == "local"` and `DevMode` (`core/internal/auth/handler.go`). `deploy/compose.prod.yml` and `deploy/compose.staging.yml` set `AUTH_DEV_MODE: false`. Internal routes require a non-empty `CONTENT_API_TOKEN` bearer (`contentTokenOK` in `core/cmd/server/mount.go`). |
| `tools/vm-acceptance.sh` present | waiting | No `tools/vm-acceptance.sh` in `7720ffc`. `docs/handoff/release1-ci-open-checks.yaml` still lists `deploy` as open for that script and the VM secrets. |
| `deploy` job green on this SHA | fail | Job `deploy` on run 45195641485 concluded `failure`. The workflow exits 1 when `tools/vm-acceptance.sh` is missing and prints that `VM_HYPERVISOR_ACCESS` and `QA_VM_SSH_HOST` are absent when those secrets are empty. |
| Clean Ubuntu VM proof (brief §8) | waiting | `docs/handoff/release1-attestation.json` at `2ea10c9` has no `vm` section. `pending_proof` lists `vm`. |
| Load pilot (D326 / brief §11) | waiting | Same attestation has no `load` section. `pending_proof` lists `load`. |
| Two independent reviews of this SHA | waiting | This document is one review. `check_reviews` in `tools/release1_done.py` requires two reviewers, `state` exactly `approved`, and `comments_closed: true`. The attestation at `2ea10c9` has no `reviews` array. |

## Findings

### Low — AU-R36 startup refusal is not on the process entrypoint

`docs/platform/auth.md` AU-R36 and its example row require the server to refuse to start when `AUTH_DEV_MODE=true` and the environment is not `local`. `auth.RejectDevMode` implements that check and `TestAU_R36_RejectDevMode` calls it. `core/cmd/server/main.go` and `rootHandler` do not call it. With `ENV=prod` and `AUTH_DEV_MODE=true`, `devStackEnabled()` is false, so the process still listens and serves `GET /healthz` only. The dev login route stays unmounted, and `loginDev` would still return 403 if it were reached with a non-local env. The unsafe login is closed; the specified fail-start is not wired.

### Low — `TestContractLiveE3` is a slice of brief §7 E3

The green contract-live job covers the lesson/answer/role path above. It does not assert an incorrect verdict on the integer answer, does not count attempts after the repeated POST, and does not call `/internal/` with and without `CONTENT_API_TOKEN`. Those internal-token checks live in other packages (`mount_s6_test.go`), outside the job’s `-run '^TestContractLiveE3$'`. The unmerged attestation’s `lesson` section describes a separate HTTP proof (including `attempt_count: 1` and foreign-tutor 403). This review did not re-execute that script.

### Info — attestation is outside the artifact

`7720ffc` does not contain `docs/handoff/release1-attestation.json`. Proof of catalog, lesson, and browser behavior is on unmerged `2ea10c9`. A new report does not replace the tree it describes. `check_ci` on that JSON still fails while `deploy` is `failure`.

## Verdict

**Approved with blockers.**

The tree at `7720ffc` is a release 1 demo on synthetic data. The ten `release1-ci` jobs other than `deploy` are green on this SHA, `TestContractLiveE3` is the contract-live command, and the Playwright task scenario asserts a 360 px viewport. Dev login is limited to the local stack, and this pass did not find pupil PII in the public tree.

The release is not finished. `deploy` fails, `tools/vm-acceptance.sh` is absent, and VM, load, and a second independent review are not attested. `python3 tools/release1_done.py` cannot return 0 on this evidence. This review is not a `check_reviews` pass: its `state` is `approved-with-blockers`, and `comments_closed` is false while the AU-R36 note is open.

## Attestation fields for JSON

```json
{
  "reviewer": "grok-4.7-medium",
  "author": "Aleksandr Mazhul",
  "sha": "7720ffc36c90c4475ea3f273bfd6b90f066ebb9e",
  "state": "approved-with-blockers",
  "requirements": [
    "release1-scope-synthetic",
    "ci-jobs-except-deploy",
    "contract-live-TestContractLiveE3",
    "e2e-playwright-360",
    "security-pii-dev-auth",
    "vm-acceptance-script",
    "deploy-job",
    "vm-proof",
    "load-pilot",
    "two-independent-reviews"
  ],
  "comments_closed": false
}
```

## Amendment (PR #72, same day)

This review targeted **`main` at `7720ffc` only**. [PR #72](https://github.com/knowgap-lab/ai-mentor/pull/72) (`1a62822` on `cursor/release1-vm-load-4c59`) adds `tools/vm-acceptance.sh`, `tools/load/*`, and `infra/terraform/` pinned to `7720ffc`; that does **not** change the evaluated artifact SHA and does **not** prove guest deploy or load. After merge, re-run `deploy` CI with VM secrets; do not copy this review into `release1-attestation.json` until a second independent review exists and AU-R36 is closed or accepted.