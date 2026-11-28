resource "libvirt_volume" "acceptance_base" {
  name = "${local.vm_domain_name}-base"
  pool = var.libvirt_pool

  source = var.ubuntu_cloud_image_url
}

resource "libvirt_volume" "acceptance_disk" {
  name           = "${local.vm_domain_name}-disk"
  pool           = var.libvirt_pool
  base_volume_id = libvirt_volume.acceptance_base.id
  size           = 20 * 1024 * 1024 * 1024
}

resource "libvirt_cloudinit_disk" "acceptance" {
  name = "${local.vm_domain_name}-cloudinit.iso"
  pool = var.libvirt_pool

  user_data = <<-EOT
#cloud-config
hostname: ${local.vm_domain_name}
users:
  - default
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: users, admin
    lock_passwd: true
    ssh_authorized_keys:
      - ${trimspace(var.acceptance_ssh_public_key)}
package_update: true
packages:
  - qemu-guest-agent
runcmd:
  - systemctl enable --now qemu-guest-agent || true
EOT
}

resource "libvirt_domain" "acceptance" {
  name   = local.vm_domain_name
  memory = var.reference_memory_mib
  vcpu   = var.reference_vcpus

  disk {
    volume_id = libvirt_volume.acceptance_disk.id
  }

  network_interface {
    network_name   = var.libvirt_network
    wait_for_lease = true
  }

  cloudinit = libvirt_cloudinit_disk.acceptance.id

  console {
    type        = "pty"
    target_port = "0"
    target_type = "serial"
  }
}