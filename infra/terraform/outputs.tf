output "app_sha" {
  value       = local.app_sha
  description = "Единственный SHA приложения. Им помечены образы core, web и content."
}

output "images" {
  value = {
    core      = local.core_image
    web       = local.web_image
    content   = local.content_image
    postgres  = local.postgres_image
    gotenberg = local.gotenberg_image
  }
  description = "Образы установки. Три образа приложения делят app_sha."
}

output "source_repository" {
  value = local.source_repository
}

output "vm_created" {
  value       = false
  description = "Корневой модуль VM не создаёт; см. acceptance-vm."
}

output "acceptance_vm_stack" {
  value       = "infra/terraform/acceptance-vm"
  description = "Отдельный Terraform-стек libvirt для одноразовой чистой Ubuntu."
}

output "reference_profile" {
  value = {
    vcpus      = var.reference_vcpus
    memory_mib = var.reference_memory_mib
    note       = "Ориентир пилота D326. Машина не создана, размер гостя не измерен."
  }
}

output "guest_label" {
  value = var.guest_label
}
