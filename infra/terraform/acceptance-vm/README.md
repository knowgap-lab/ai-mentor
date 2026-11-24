# Стек libvirt для приёмочной Ubuntu (D321, D340)

Отдельное состояние Terraform: **только** домен `ai-mentor-accept-*`. Пользовательские VM не затрагиваются.

Запуск с KVM-хоста (не из job `deploy` без секретов):

```bash
export TF_VAR_acceptance_ssh_public_key="$(cat ~/.ssh/id_ed25519.pub)"
export LIBVIRT_DEFAULT_URI="${LIBVIRT_DEFAULT_URI:-qemu:///system}"
cd infra/terraform/acceptance-vm
terraform init -input=false
terraform apply -auto-approve -input=false
terraform plan -detailed-exitcode -input=false  # ожидается exit 0
```

Обёртка: `bash tools/vm-hypervisor-libvirt-apply.sh` из корня репозитория.