locals {
  labels = {
    cluster    = var.cluster_name
    managed_by = "opentofu"
  }
}

resource "hcloud_network" "cluster" {
  name     = "${var.cluster_name}-network"
  ip_range = var.network_cidr
  labels   = local.labels
}

resource "hcloud_network_subnet" "cluster" {
  network_id   = hcloud_network.cluster.id
  type         = "cloud"
  network_zone = var.network_zone
  ip_range     = var.subnet_cidr
}

resource "hcloud_ssh_key" "operator" {
  name       = "${var.cluster_name}-operator"
  public_key = file(pathexpand(var.ssh_public_key_path))
  labels     = local.labels
}

resource "hcloud_firewall" "cluster" {
  name   = "${var.cluster_name}-nodes"
  labels = local.labels

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = var.admin_cidrs
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "6443"
    source_ips = var.admin_cidrs
  }

  rule {
    direction  = "in"
    protocol   = "icmp"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  dynamic "rule" {
    for_each = ["6443", "9345", "10250", "30000-32767"]
    content {
      direction  = "in"
      protocol   = "tcp"
      port       = rule.value
      source_ips = [var.network_cidr]
    }
  }

  rule {
    direction  = "in"
    protocol   = "udp"
    port       = "8472"
    source_ips = [var.network_cidr]
  }
}

resource "hcloud_server" "control_plane" {
  name         = "${var.cluster_name}-control-1"
  image        = var.image
  server_type  = var.control_plane_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.operator.id]
  firewall_ids = [hcloud_firewall.cluster.id]
  user_data    = file("${path.module}/cloud-init.yaml")
  labels       = merge(local.labels, { role = "control-plane" })

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  network {
    network_id = hcloud_network.cluster.id
    ip         = var.control_plane_ip
  }

  depends_on = [hcloud_network_subnet.cluster]
}

resource "hcloud_server" "system_worker" {
  name         = "${var.cluster_name}-system-1"
  image        = var.image
  server_type  = var.system_worker_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.operator.id]
  firewall_ids = [hcloud_firewall.cluster.id]
  user_data    = file("${path.module}/cloud-init.yaml")
  labels       = merge(local.labels, { role = "system-worker" })

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  network {
    network_id = hcloud_network.cluster.id
    ip         = var.system_worker_ip
  }

  depends_on = [hcloud_network_subnet.cluster]
}
