check "app_sha_is_one_commit" {
  assert {
    condition     = can(regex("^[0-9a-f]{40}$", local.app_sha))
    error_message = "app_sha (-var / TF_VAR_app_sha) должен быть одним полным SHA коммита под тестом."
  }
}

check "application_images_share_that_sha" {
  assert {
    condition = (
      endswith(local.core_image, ":${local.app_sha}") &&
      endswith(local.web_image, ":${local.app_sha}") &&
      endswith(local.content_image, ":${local.app_sha}") &&
      !strcontains(local.core_image, "latest") &&
      !strcontains(local.web_image, "latest") &&
      !strcontains(local.content_image, "latest") &&
      !strcontains(local.postgres_image, "latest") &&
      !strcontains(local.gotenberg_image, "latest") &&
      can(regex(local.image_repository_pattern, local.core_image_repository)) &&
      can(regex(local.image_repository_pattern, local.web_image_repository)) &&
      can(regex(local.image_repository_pattern, local.content_image_repository))
    )
    error_message = "Образы core, web и content берут один SHA из release.json. Тег latest запрещён (OP-R9)."
  }
}

check "third_party_images_match_compose" {
  assert {
    condition = (
      local.postgres_image == "postgres:16.15" &&
      local.gotenberg_image == "gotenberg/gotenberg:9.5.0"
    )
    error_message = "Postgres и Gotenberg остаются версиями из deploy/compose.prod.yml."
  }
}

check "hypervisor_not_selected" {
  assert {
    condition     = var.create_vm == false
    error_message = "Гипервизор не выбран (D321). create_vm нельзя применять. Установка Ansible на уже созданную Ubuntu эту проверку не закрывает."
  }
}
