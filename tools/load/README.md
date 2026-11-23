# Нагрузочный пилот D326 (выпуск 1)

Профиль: 20 синтетических учеников, 2 репетитора, 4 vCPU / 8 GiB с наблюдаемостью; разогрев ≥ 300 с, измерение ≥ 1200 с; p95 чтения ≤ 500 мс, записи ≤ 1000 мс (`tools/load/d326-profile.json`).

## Команды

```bash
# Сверка профиля без HTTP и без записи pass в аттестацию
python3 tools/load/release1_load_pilot.py --validate-config

# Короткая проверка исполнителя (не заменяет 20-минутный замер, не для CI)
python3 tools/load/release1_load_pilot.py --smoke --origin http://127.0.0.1:8080

# Полный пилот — только на выделенной VM после tools/vm-acceptance.sh; в CI запрещён
RELEASE_LOAD_PILOT_CONFIRM=1 LOAD_PILOT_VIRTUALIZATION=kvm \
  python3 tools/load/release1_load_pilot.py --measure --origin "https://prod.example"
```

Вывод `--measure` — JSON на stdout для ручной проверки и последующей записи в аттестацию **после** успешного прогона на VM. Скрипт сам не пишет `pass` в `release1-attestation.json`.

## Зависимости

- Поднятое приложение на принятом SHA с синтетическими учётками (`AUTH_DEV_MODE` только на стенде, не в прод-приёмке).
- Переменная `LOAD_PILOT_VIRTUALIZATION` не должна быть `cloud_container` или `cursor_cloud`.
- В GitHub Actions полный `--measure` завершается с кодом 2 (заблокирован).