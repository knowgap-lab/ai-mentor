locals {
  release = jsondecode(file("${path.module}/release.json"))

  # SHA коммита под тестом: TF_VAR_app_sha / -var=app_sha (см. tools/verify_release_sha.sh).
  app_sha                  = var.app_sha
  source_repository        = local.release.source_repository
  core_image_repository    = local.release.core_image_repository
  web_image_repository     = local.release.web_image_repository
  content_image_repository = local.release.content_image_repository

  core_image    = "${local.core_image_repository}:${local.app_sha}"
  web_image     = "${local.web_image_repository}:${local.app_sha}"
  content_image = "${local.content_image_repository}:${local.app_sha}"

  # Цитата deploy/compose.prod.yml: сервисы db и gotenberg. Ansible их не переопределяет.
  postgres_image  = "postgres:16.15"
  gotenberg_image = "gotenberg/gotenberg:9.5.0"

  image_repository_pattern = "^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+$"

  release_contract = {
    app_sha           = local.app_sha
    source_repository = local.source_repository
    images = {
      core      = local.core_image
      web       = local.web_image
      content   = local.content_image
      postgres  = local.postgres_image
      gotenberg = local.gotenberg_image
    }
    guest_label          = var.guest_label
    reference_vcpus      = var.reference_vcpus
    reference_memory_mib = var.reference_memory_mib
    vm_created           = false
  }
}
