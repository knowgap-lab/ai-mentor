check "ssh_key_required" {
  assert {
    condition = (
      length(trimspace(var.acceptance_ssh_public_key)) > 0 &&
      can(regex("^ssh-", trimspace(var.acceptance_ssh_public_key)))
    )
    error_message = "acceptance_ssh_public_key обязателен (одна строка ssh-*)."
  }
}