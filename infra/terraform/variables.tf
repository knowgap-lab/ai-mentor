variable "create_vm" {
  type        = bool
  default     = false
  description = "Создание VM выключено: гипервизор не выбран. true план отвергает."
}

variable "guest_label" {
  type        = string
  default     = "existing-ubuntu-guest"
  description = "Метка уже существующей гостевой Ubuntu. Машину не создаёт."

  validation {
    condition     = can(regex("^[a-z0-9-]{1,32}$", var.guest_label))
    error_message = "guest_label: строчные латинские буквы, цифры и дефис, до 32 символов."
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
