variable "app_sha" {
  type        = string
  description = "SHA проверяемого коммита (RELEASE_SHA / GITHUB_SHA / git rev-parse HEAD)."

  validation {
    condition     = can(regex("^[0-9a-f]{40}$", var.app_sha))
    error_message = "app_sha должен быть одним полным SHA коммита."
  }
}

variable "create_vm" {
  type        = bool
  default     = false
  description = "Создание VM выключено: гипервизор не выбран. true план отвергает."
}

variable "guest_label" {
  type        = string
  default     = "ai-mentor-accept-guest"
  description = "Суффикс имени домена libvirt (ai-mentor-accept-<label>). Только эта VM управляется Terraform."

  validation {
    condition     = can(regex("^ai-mentor-accept-[a-z0-9-]{1,20}$", var.guest_label))
    error_message = "guest_label должен быть вида ai-mentor-accept-<суффикс>, только строчные латиница, цифры, дефис."
  }
}

variable "reference_vcpus" {
  type        = number
  default     = 4
  description = "Ориентир D326, 4 vCPU. Не параметр созданной VM."

  validation {
    condition     = var.reference_vcpus == 4
    error_message = "Ориентир D326 — 4 vCPU. Другой размер записывается отдельным решением."
  }
}

variable "reference_memory_mib" {
  type        = number
  default     = 8192
  description = "Ориентир D326, 8 ГиБ. Не параметр созданной VM."

  validation {
    condition     = var.reference_memory_mib == 8192
    error_message = "Ориентир D326 — 8192 МиБ. Другой размер записывается отдельным решением."
  }
}
