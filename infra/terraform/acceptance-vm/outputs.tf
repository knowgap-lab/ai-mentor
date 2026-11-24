output "acceptance_domain" {
  value       = local.vm_domain_name
  description = "Имя домена libvirt."
}

output "acceptance_ipv4" {
  value       = try(libvirt_domain.acceptance.network_interface[0].addresses[0], null)
  description = "DHCP IPv4 для QA_VM_SSH_HOST."
}

output "vm_created" {
  value       = true
  description = "Стек создаёт одну приёмочную VM."
}