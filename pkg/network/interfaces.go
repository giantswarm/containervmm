/*

Copyright 2020 Salvatore Mazzarino

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package network

import (
	"fmt"
	"net"

	"github.com/mazzy89/containervmm/pkg/api"

	"github.com/mazzy89/containervmm/pkg/util"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// Array of container interfaces to ignore (not forward to vm)
var ignoreInterfaces = map[string]struct{}{
	"lo": {},
}

func SetupInterfaces(guest *api.Guest) ([]DHCPInterface, error) {
	var dhcpIfaces []DHCPInterface
	var nics []api.NetworkInterface

	netHandle, err := netlink.NewHandle()
	if err != nil {
		return nil, err
	}
	defer netHandle.Delete()

	ifaces, err := net.Interfaces()
	if err != nil || ifaces == nil || len(ifaces) == 0 {
		return nil, fmt.Errorf("cannot get local network interfaces: %v", err)
	}

	interfacesCount := 0
	for _, iface := range ifaces {
		// Skip the interface if it's ignored
		if _, ok := ignoreInterfaces[iface.Name]; ok {
			continue
		}

		// Try to transfer the address from the container to the DHCP server
		ipNet, gw, routes, _, err := takeAddress(netHandle, &iface)
		if err != nil {
			// Log the problem, but don't quit the function here as there might be other good interfaces
			log.Errorf("parsing interface %q failed: %w", iface.Name, err)

			// Try with the next interface
			continue
		}

		dhcpIface, err := bridge(netHandle, &iface)
		if err != nil {
			// Log the problem, but don't quit the function here as there might be other good interfaces
			// Don't set shouldRetry here as there is no point really with retrying with this interface
			// that seems broken/unsupported in some way.
			log.Errorf("bridging interface %q failed: %v", iface.Name, err)
			// Try with the next interface
			continue
		}

		dhcpIface.VMIPNet = ipNet
		dhcpIface.GatewayIP = gw
		dhcpIface.Routes = routes

		dhcpIfaces = append(dhcpIfaces, *dhcpIface)

		// bind DHCP Network Interfaces to the Guest object
		nics = append(nics, api.NetworkInterface{
			GatewayIP:   gw,
			InterfaceIP: &ipNet.IP,
			Routes:      routes,
			MacAddr:     dhcpIface.MACFilter,
			TAP:         dhcpIface.VMTAP,
		})

		// This is an interface we care about
		interfacesCount++
	}

	if interfacesCount == 0 {
		return nil, fmt.Errorf("no active or valid interfaces available yet")
	}

	guest.NICs = nics

	return dhcpIfaces, nil
}

// takeAddress removes the first address of an interface and returns it and the appropriate gateway
func takeAddress(netHandle *netlink.Handle, iface *net.Interface) (*net.IPNet, *net.IP, []netlink.Route, bool, error) {
	addrs, err := iface.Addrs()
	if err != nil || addrs == nil || len(addrs) == 0 {
		// set the bool to true so the caller knows to retry
		return nil, nil, nil, true, fmt.Errorf("interface %q has no address", iface.Name)
	}

	for _, addr := range addrs {
		var ip net.IP
		var mask net.IPMask

		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
			mask = v.Mask
		case *net.IPAddr:
			ip = v.IP
			mask = ip.DefaultMask()
		}

		if ip == nil {
			continue
		}

		ip = ip.To4()
		if ip == nil {
			continue
		}

		link, err := netHandle.LinkByName(iface.Name)
		if err != nil {
			return nil, nil, nil, false, fmt.Errorf("failed to get interface %q by name: %v", iface.Name, err)
		}

		var gw *net.IP
		routes, err := netHandle.RouteList(link, netlink.FAMILY_ALL)
		if err != nil {
			return nil, nil, nil, false, fmt.Errorf("failed to get default gateway for interface %q: %v", iface.Name, err)
		}
		for _, rt := range routes {
			if rt.Gw != nil {
				gw = &rt.Gw
				break
			}
		}

		delAddr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   ip,
				Mask: mask,
			},
		}
		if err = netHandle.AddrDel(link, delAddr); err != nil {
			return nil, nil, nil, false, fmt.Errorf("failed to remove address %q from interface %q: %v", delAddr, iface.Name, err)
		}

		log.Infof("Moving IP address %s (%s) with gateway %s from container to guest", ip.String(), maskString(mask), gw.String())

		return &net.IPNet{
			IP:   ip,
			Mask: mask,
		}, gw, routes, false, nil
	}

	return nil, nil, nil, false, fmt.Errorf("interface %s has no valid addresses", iface.Name)
}

// bridge creates the TAP device and performs the bridging, returning the base configuration for a DHCP server
func bridge(netHandle *netlink.Handle, iface *net.Interface) (*DHCPInterface, error) {
	tapName := "tap-" + iface.Name
	bridgeName := "br-" + iface.Name

	eth, err := netHandle.LinkByIndex(iface.Index)
	if err != nil {
		return nil, err
	}

	// Move the veth address to the TAP interface. This MAC address has to be
	// the one inside the VM in order to avoid any firewall issues. The
	// bridge created by the network plugin on the host actually expects
	// to see traffic from this MAC address and not another one.
	tapHardAddr := eth.Attrs().HardwareAddr

	// Generate the MAC addresses for the VM's adapters
	randomMacAddr, err := util.GenerateRandomPrivateMacAddr()
	if err != nil {
		return nil, fmt.Errorf("failed to generate MAC addresses: %v", err)
	}

	hardAddr, err := net.ParseMAC(randomMacAddr)
	if err != nil {
		return nil, err
	}

	if err := netHandle.LinkSetHardwareAddr(eth, hardAddr); err != nil {
		return nil, fmt.Errorf("failed to set MAC address %s for eth interface %s: %s",
			randomMacAddr, eth.Attrs().Name, err)
	}

	tuntap, err := createTAPAdapter(netHandle, tapName, tapHardAddr)
	if err != nil {
		return nil, fmt.Errorf("creation tap interface %q failed: %w", tapName, err)
	}

	bridge, err := createBridge(netHandle, bridgeName)
	if err != nil {
		return nil, fmt.Errorf("creation bridge %q failed: %w", bridgeName, err)
	}

	if err := setMaster(netHandle, bridge, tuntap, eth); err != nil {
		return nil, fmt.Errorf("failed to set master: %v", err)
	}

	return &DHCPInterface{
		VMTAP:  tapName,
		Bridge: bridgeName,
		// Set the MAC address filter for the DHCP server
		MACFilter: tapHardAddr.String(),
	}, nil
}

// createTAPAdapter creates a new TAP device with the given name
func createTAPAdapter(netHandle *netlink.Handle, tapName string, hardAddr net.HardwareAddr) (*netlink.Tuntap, error) {
	la := netlink.NewLinkAttrs()
	la.Name = tapName
	la.HardwareAddr = hardAddr

	tuntap := &netlink.Tuntap{
		LinkAttrs: la,
		Mode:      netlink.TUNTAP_MODE_TAP,
	}

	return tuntap, addLink(netHandle, tuntap)
}

// createBridge creates a new bridge device with the given name
func createBridge(netHandle *netlink.Handle, bridgeName string) (*netlink.Bridge, error) {
	la := netlink.NewLinkAttrs()
	la.Name = bridgeName

	// Disable MAC address age tracking. This causes issues in the container,
	// the bridge is unable to resolve MACs from outside resulting in it never
	// establishing the internal routes. This "optimization" is only really useful
	// with more than two interfaces attached to the bridge anyways, so we're not
	// taking any performance hit by disabling it here.
	ageingTime := uint32(0)
	bridge := &netlink.Bridge{LinkAttrs: la, AgeingTime: &ageingTime}
	return bridge, addLink(netHandle, bridge)
}

// addLink creates the given link and brings it up
func addLink(netHandle *netlink.Handle, link netlink.Link) (err error) {
	if err = netHandle.LinkAdd(link); err == nil {
		err = netHandle.LinkSetUp(link)
	}

	return
}

func setMaster(netHandle *netlink.Handle, master netlink.Link, links ...netlink.Link) error {
	masterIndex := master.Attrs().Index
	for _, link := range links {
		if err := netHandle.LinkSetMasterByIndex(link, masterIndex); err != nil {
			return err
		}
	}

	return nil
}

func maskString(mask net.IPMask) string {
	if len(mask) < 4 {
		return "<nil>"
	}

	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}
