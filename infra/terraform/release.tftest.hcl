run "pins_one_sha" {
  command = plan

  assert {
    condition     = can(regex("^[0-9a-f]{40}$", output.app_sha))
    error_message = "plan должен отдавать app_sha из -var=app_sha / TF_VAR_app_sha."
  }

  assert {
    condition = (
      output.images.core == "ghcr.io/knowgap-lab/ai-mentor-core:${output.app_sha}" &&
      output.images.web == "ghcr.io/knowgap-lab/ai-mentor-web:${output.app_sha}" &&
      output.images.content == "ghcr.io/knowgap-lab/ai-mentor-content:${output.app_sha}"
    )
    error_message = "Образы приложения должны ссылаться на один SHA."
  }

  assert {
    condition     = output.images.postgres == "postgres:16.15" && output.images.gotenberg == "gotenberg/gotenberg:9.5.0"
    error_message = "Сторонние образы должны совпадать с compose."
  }

  assert {
    condition     = output.vm_created == false
    error_message = "Конфигурация не должна сообщать, что VM создана."
  }
}

run "create_vm_is_refused" {
  command = plan

  variables {
    create_vm = true
  }

  expect_failures = [
    check.hypervisor_not_selected,
  ]
}
