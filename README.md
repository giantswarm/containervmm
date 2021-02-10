# Container Virtual Machine Manager

Run Virtual Machines and manage their lifecycle inside a Docker container.

## Requirements

* Docker
* KVM Support

## Features

- Configure either via CLI flags or environment variables
- Ability to configure VM resources (CPUs, memory, disk size)
- Mount additional disks
- Mount host volumes
- OS network configuration automatically handled by DHCP
- Set custom DNS and NTP servers
- Load Ignition to configure the OS

## Usage

```shell
Container Virtual Machine Manager spins up a Virtual Machine inside a container

Usage:
  containervmm [options] [flags]

Examples:
containervmm --flatcar-version=2605.6.0

Flags:
      --debug                            enable debug
      --flatcar-channel string           flatcar channel (i.e. stable, beta, alpha) (default "stable")
      --flatcar-ignition string          base64-encoded Ignition Config
      --flatcar-ignition-dir string      dir path of the Ignition config (default "/")
      --flatcar-version string           flatcar version
      --guest-additional-disks strings   guest additional disk to mount (i.e. "dockerfs:20GB")
      --guest-cpus string                guest cpus (default "1")
      --guest-dns-servers strings        guest DNS Servers. If left empty, the DNS servers given are the one of the container
      --guest-host-volumes strings       guest host volume (i.e. "datashare:/usr/data")
      --guest-memory string              guest memory (default "1024M")
      --guest-name string                guest name (default "flatcar_production_qemu")
      --guest-ntp-servers strings        guest NTP Servers. If left empty, the NTP servers set are the default one from the distro
      --guest-root-disk-size string      guest root disk size (default "20G")
  -h, --help                             help for containervmm
      --sanity-checks                    run sanity checks (GPG verification of images) (default true)
  ```

## Hypervisor supported

* QEMU - Quick EMUlator
* AWS Firecracker (Coming)

## OS/Arch Supported

- Flatcar Container Linux by Kinvolk (`amd64`)

## Installation

Run:

```sh
docker run -it --rm --device /dev/kvm:/dev/kvm --device /dev/net/tun:/dev/net/tun containervmm --flatcar-version=2605.6.0
```
