# For Review — решения карточек трекера

**Дата:** 2026-11-24  
**Main:** `a35f8b0489cd355501e40d187d3a0231a1bd3077`  
**Ветка записи:** `cursor/review-card-decisions`  
**Метод:** `git merge-base origin/main <pr-head>`, `gh pr view`, `gh pr checks`, `python3 tools/release1_done.py` на дереве main (без Docker).

Карточки трекера этой сессией **не закрывались** — только документированное решение для ревьюера.

| Card | Decision | SHA | Proof |
|------|----------|-----|-------|
| AM-37 | landed via **PR #6** (`e125b6b`); PR [#20](https://github.com/knowgap-lab/ai-mentor/pull/20) obsolete vs main | `a35f8b0` satisfies; `329e28f` stale | [main code-check](https://github.com/knowgap-lab/ai-mentor/actions/runs/45252840133/job/137966757912) |
| AM-43 | still open | `a35f8b0` smoke **fail** (install 100, start/check 1); PR #57 still `dc1e160` only | [evidence JSON](../../evidence/cloud-vm/a35f8b0489cd355501e40d187d3a0231a1bd3077.json) |
| AM-64 | still open | `4b8443a` on PR #69; fails on main | [PR #69 code-check](https://github.com/knowgap-lab/ai-mentor/actions/runs/45254843617/job/137973828108) |
| AM-59 | still open | `a35f8b0` CI minus deploy; record at `4b8443a` | [deploy fail on main](https://github.com/knowgap-lab/ai-mentor/actions/runs/45252839895/job/137966757266) |
| AM-55 | obsolete (superseded by PR #69) | `df704a0` / `f211cf3` | [PR #63](https://github.com/knowgap-lab/ai-mentor/pull/63) |
| AM-27 | obsolete (checker on main) | `a35f8b0` satisfies; `2a0f88c` fails CI | [PR #17 code-check fail](https://github.com/knowgap-lab/ai-mentor/actions/runs/43697924999/job/132762471738) |
| AM-28 | obsolete | `b979274` stale vs main handoff | [PR #12](https://github.com/knowgap-lab/ai-mentor/pull/12) |
| AM-21 | still open | `a35f8b0` checker exit 1 | `python3 tools/release1_done.py` → `first_failing_check: pdf` |
| AM-20 | landed via **PR #6**; PR #8 obsolete | `a35f8b0` journal pass | [PR #8](https://github.com/knowgap-lab/ai-mentor/pull/8) green but stale base |
| AM-19 | landed via **PR #6** / #10; PR #7 obsolete | `a35f8b0` (CI); local pdf fail without Docker | [main code-check](https://github.com/knowgap-lab/ai-mentor/actions/runs/45252840133/job/137966757912) |
| AM-1 | rollup — **still open** (do not close) | unfinished `a35f8b0` | [release1-ci on main](https://github.com/knowgap-lab/ai-mentor/actions/runs/45252839895) |

## Детали по приоритетным карточкам

### AM-37 — Add release observability

- **merge-base**(`main`, PR #20 `329e28f`): `ad16958`.
- На main: `deploy/ops-alert.sh`, `disk-check.sh`, `status-check.sh`, `log-retain.sh`, `observability_test.py` с коммита `e125b6b` (PR #6).
- `tools/release1_done.py` на `a35f8b0`: `observability: pass`.
- PR #20 не влит; относительно main удаляет release1-артефакты — **не мержить без перебазы**.

### AM-43 — Check candidate cloud environment

- **merge-base**(`main`, `f833246`): `ad16958`.
- PR [#57](https://github.com/knowgap-lab/ai-mentor/pull/57) — только `dc1e160` (все три команды exit 0).
- Прогон 2026-11-24 на `a35f8b0` (Cursor Cloud Agent VM, detached worktree): `dev-install` **100** (нет deb `google-chrome-stable` `158.0.8127.36-1`), `dev-start` **1**, `dev-check` **1** (нет доступа к Docker без группы `docker`). Журналы: `evidence/cloud-vm/logs/a35f8b0489cd355501e40d187d3a0231a1bd3077/`.
- **Следующая команда:** `sg docker -c 'bash tools/dev-install.sh && bash tools/dev-start.sh && bash tools/dev-check.sh'` на VM, где apt отдаёт pinned Chrome, либо обновить pin в окружении.

### AM-64 / AM-59 / AM-55

- Аттестация и полный CI-журнал для приёмки — голова PR [#69](https://github.com/knowgap-lab/ai-mentor/pull/69) `4b8443a` (PR #69 в этой сессии не менялся).
- На main: все job release1-ci **кроме** [deploy](https://github.com/knowgap-lab/ai-mentor/actions/runs/45252839895/job/137966757266) — success на `a35f8b0`.
- AM-55: PR #62/#63 заменены PR #69.
- `docs/handoff/release1-attestation.json` **не** входит в main (D142 / процесс приёмки).

### AM-27 / AM-28

- Устаревшие док-/tool PR относительно main после PR #6 и последующих handoff-записей.

### AM-21 / AM-20 / AM-19

- AM-21: единый SHA с `release1_done.py` exit 0 ещё нет на main.
- AM-20, AM-19: функциональность на main; открытые PR #8/#7 закрыть как obsolete.

### AM-1 (rollup)

Десять дочерних карточек в For Review: **3** satisfied on main through PR #6 (AM-37, AM-20, AM-19), **4** obsolete PR (#55, #27, #28 + stale #20/#7/#17/#12), **5** still need work (AM-43, AM-64, AM-59, AM-21, parent criteria). Блокеры: VM deploy, attestation merge policy, second reviewer, полный checker exit 0.