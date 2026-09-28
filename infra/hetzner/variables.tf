variable "cluster_name" {
  type    = string
  default = "adk-research"
}

variable "location" {
  type    = string
  default = "nbg1"
}

variable "network_zone" {
  type    = string
  default = "eu-central"
}

variable "network_cidr" {
  type    = string
  default = "10.20.0.0/16"
}

variable "subnet_cidr" {
  type    = string
  default = "10.20.0.0/24"
}

variable "control_plane_ip" {
  type    = string
  default = "10.20.0.10"
}

variable "system_worker_ip" {
  type    = string
  default = "10.20.0.20"
}

variable "control_plane_type" {
  type    = string
  default = "cpx22"
}

variable "system_worker_type" {
  type    = string
  default = "cpx22"
}

variable "image" {
  type    = string
  default = "ubuntu-24.04"
}

variable "ssh_public_key_path" {
  type = string
}

variable "admin_cidrs" {
  type = list(string)

  validation {
    condition     = length(var.admin_cidrs) > 0
    error_message = "admin_cidrs must contain at least one trusted CIDR"
  }
}
