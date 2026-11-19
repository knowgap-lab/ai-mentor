# Контракт выпуска 1. Ресурса виртуализации нет: бэкенд не выбран.
resource "terraform_data" "release" {
  input = local.release_contract
}
