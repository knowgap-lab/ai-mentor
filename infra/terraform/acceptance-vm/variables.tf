variable "guest_label" {
  type        = string
  default     = "ai-mentor-accept-guest"
  description = "Имя домена libvirt (ai-mentor-accept-*). Управляется только этим стеком."

  validation {
    condition     = can(regex("^ai-mentor-accept-[a-z0-9-]{1,20}$", var.guest_label))
    error_message = "guest_label должен быть вида ai-mentor-accept-<суффикс>."
  }
}

variable "libvirt_uri" {
  type        = string
  default     = "qemu:///system"
  description = "URI libvirt на KVM-хосте оператора."

  validation {
    condition     = can(regex("^qemu:///", var.libvirt_uri)) || can(regex("^qemu\\+ssh://", var.libvirt_uri))
    error_message = "libvirt_uri: qemu:///… или qemu+ssh://…"
  }
}

variable "libvirt_pool" {
  type    = string
  default = "default"
}

variable "libvirt_network" {
  type    = string
  default = "default"
}

variable "ubuntu_cloud_image_url" {
  type        = string
  default     = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  description = "Официальный cloud image Ubuntu 24.04 noble."
}

variable "acceptance_ssh_public_key" {
  type        = string
  sensitive   = true
  description = "Публичный ключ SSH для cloud-init (пользователь ubuntu)."
}

variable "reference_vcpus" {
  type    = number
  default = 4
}

variable "reference_memory_mib" {
  type    = number
  default = 8192
}