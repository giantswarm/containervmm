# Container VMM

Run VMs inside a Docker container.

## Requirements

* Docker
* KVM Support

## Hypervisor supported

* QEMU

## OS Supported

* Flatcar Linux

## Installation

Run:

```sh
docker run -it --rm --device /dev/kvm:/dev/kvm --device /dev/net/tun:/dev/net/tun --cap-add NET_ADMIN containervmm --flatcar-version=2605.6.0
```
