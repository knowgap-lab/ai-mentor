# Terraform выпуска 1

Один файл `release.json` задаёт репозитории образов. Образы `core`, `web` и `content` получают SHA коммита под тестом (`-var app_sha` / `TF_VAR_app_sha`, D339). Postgres `16.15` и Gotenberg `9.5.0` взяты из `deploy/compose.prod.yml`.

Корневой модуль — **контракт** выпуска (образы, ориентир D326). VM **не** создаётся здесь (`create_vm` всегда false).

Отдельная чистая Ubuntu на KVM/libvirt — стек [acceptance-vm/](acceptance-vm/) (D321, D340). Только имена `ai-mentor-accept-*`.

## Команды (контракт, без гипервизора)

Из этого каталога:

```bash
export TF_VAR_app_sha="$(git -C ../.. rev-parse HEAD^{commit})"
terraform fmt -check -recursive
terraform init -backend=false -input=false
terraform validate
terraform test
terraform apply -auto-approve -input=false -var="create_vm=false"
terraform plan -detailed-exitcode -input=false -var="create_vm=false"
```

## Гостевая VM (машина оператора с libvirt)

```bash
bash ../../tools/vm-hypervisor-libvirt-apply.sh
cd acceptance-vm && terraform plan -detailed-exitcode -input=false
```

Второй `plan` по тому же стеку `acceptance-vm` — только exit 0.

`terraform.tfstate` в git не входит.