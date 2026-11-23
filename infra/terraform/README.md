# Terraform выпуска 1

Один файл `release.json` задаёт SHA приложения. Образы `core`, `web` и `content` получают этот SHA тегом. Postgres `16.15` и Gotenberg `9.5.0` взяты из `deploy/compose.prod.yml` и здесь не подменяются.

Конфигурация не создаёт VM. Гипервизор не выбран (D321): `create_vm` по умолчанию `false`, значение `true` план отвергает. Проверка `terraform validate` не означает, что VM создана или что установка на гостевую Ubuntu выполнена.

Ориентир 4 vCPU и 8 ГиБ записан по D326. Это не измеренный гость и не созданная машина.

## Команды

Из этого каталога:

```bash
terraform fmt -check -recursive
terraform init
terraform validate
terraform test
terraform apply -auto-approve -input=false
terraform plan -detailed-exitcode -input=false
```

Повторный `plan` после `apply` того же контракта должен завершиться с кодом 0: изменений нет. `terraform.tfstate` остаётся в этом каталоге и в git не входит.

`create_vm=true` применять нельзя, пока после осмотра компьютера пользователя не выбран бэкенд виртуализации. Установка Ansible на уже существующую Ubuntu эту проверку не закрывает.
