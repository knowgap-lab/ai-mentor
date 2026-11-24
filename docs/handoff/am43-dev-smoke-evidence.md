# AM-43: dev smoke (синтетические данные)

Evaluated **`origin/main`** после squash PR #73: `c6d9603c63f4c366badacdeea47904de2fbdff0d`.

## Не pass (запись на PR #74, `c9f21f99`, дерево `a35f8b0`)

| Команда | Код | Причина |
|---------|-----|---------|
| `tools/dev-install.sh` | 100 | `E: Version '158.0.8127.36-1' for 'google-chrome-stable' was not found` (`mise.toml` CHROME_VERSION) |
| `tools/dev-start.sh` | 1 | Нет штатного доступа к Docker (пользователь не в группе `docker` после install) |
| `tools/dev-check.sh` | 1 | То же (Docker) |

Это **не** attestation pass. Chrome и проверку Docker не ослабляли (D320).

## После `fix(dev): pin google-chrome-stable` (`d065a9e` на `cursor/chrome-apt-pin-am43-4c59`)

Пин: `CHROME_VERSION = 158.0.8127.88-1`, SHA256 проверен при `apt-get download`.

| Команда | Код | Примечание |
|---------|-----|------------|
| `tools/dev-install.sh` | 0 | Ubuntu cloud VM; `usermod -aG docker` из скрипта |
| `tools/dev-start.sh` | 0 | `sg docker -c 'bash tools/dev-start.sh'` (новая сессия с группой) |
| `tools/dev-check.sh` | 0 | `sg docker -c 'bash tools/dev-check.sh'` |

Pass только при всех трёх кодах **0**.