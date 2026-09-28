output "control_plane" {
  value = {
    name       = hcloud_server.control_plane.name
    public_ip  = hcloud_server.control_plane.ipv4_address
    private_ip = var.control_plane_ip
  }
}

output "system_worker" {
  value = {
    name       = hcloud_server.system_worker.name
    public_ip  = hcloud_server.system_worker.ipv4_address
    private_ip = var.system_worker_ip
  }
}

output "network_id" {
  value = hcloud_network.cluster.id
}

output "firewall_id" {
  value = hcloud_firewall.cluster.id
}

output "ssh_key_id" {
  value = hcloud_ssh_key.operator.id
}

output "location" {
  value = var.location
}
