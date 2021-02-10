package api

import (
	"net"

	"github.com/vishvananda/netlink"
)

// Guest describes the configuration of a VM
// created and run by QEMU
type Guest struct {
	Name string

	CPUs   string
	Memory string

	Disks       []Disk
	HostVolumes []HostVolume

	// Guest OS
	OS OS

	// DHCP Interfaces
	NICs []NetworkInterface
}

// OS describe sthe configuration of the OS
type OS struct {
	Kernel string
	Initrd string

	IgnitionConfig string
}

// NetworkInterface describe the network interface of the guest
type NetworkInterface struct {
	GatewayIP   *net.IP
	InterfaceIP *net.IP
	Routes      []netlink.Route
	MacAddr     string
	TAP         string
}

type FsType string

const (
	// filesystem type
	XFS  FsType = "xfs"
	EXT4 FsType = "ext4"
)

type Disk struct {
	ID string

	Size   string
	File   string
	IsRoot bool

	Filesystem FsType
}

// HostVolume is a shared volume between the host and the VM,
// defined by its mount tag and its host path.
type HostVolume struct {
	// MountTag is a label used as a hint to the guest.
	MountTag string

	// HostPath is the host filesystem path for this volume.
	HostPath string
}
