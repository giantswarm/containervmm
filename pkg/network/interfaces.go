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
	"os"
	"syscall"

	"github.com/giantswarm/containervmm/pkg/api"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// Array of container interfaces to ignore (not forward to vm)
var ignoreInterfaces = map[string]struct{}{
	"lo":    {},
	"tunl0": {},
}

const TAP_PREFIX = "tap-"

func SetupContainerNetworking(guest *api.Guest) error {
	var vmIntfs []api.NetworkInterface

	netHandle, err := netlink.NewHandle()
	if err != nil {
		return err
	}
	defer netHandle.Delete()

	ifaces, err := net.Interfaces()
	if err != nil || ifaces == nil || len(ifaces) == 0 {
		return fmt.Errorf("cannot get local network interfaces: %v", err)
	}

	interfacesCount := 0
	for _, iface := range ifaces {
		// Skip the interface if it's ignored
		if _, ok := ignoreInterfaces[iface.Name]; ok {
			continue
		}

		err := addTcRedirect(netHandle, &iface)
		if err != nil {
			return err
		}

		vmIntfs = append(vmIntfs, api.NetworkInterface{
			MacAddr: iface.HardwareAddr.String(),
			TAP:     TAP_PREFIX + iface.Name,
		})

		// This is an interface we care about
		interfacesCount++
	}

	if interfacesCount == 0 {
		return fmt.Errorf("no active or valid interfaces available yet")
	}

	guest.NICs = vmIntfs

	return nil
}

func addTcRedirect(netHandle *netlink.Handle, iface *net.Interface) error {

	log.Infof("Adding tc-redirect for %q", iface.Name)

	eth, err := netHandle.LinkByIndex(iface.Index)
	if err != nil {
		return err
	}

	tapName := TAP_PREFIX + iface.Name
	tuntap, err := createTAPAdapter(netHandle, tapName)
	if err != nil {
		return err
	}

	err = addIngressQdisc(netHandle, eth)
	if err != nil {
		return err
	}
	err = addIngressQdisc(netHandle, tuntap)
	if err != nil {
		return err
	}

	err = addRedirectFilter(netHandle, eth, tuntap)
	if err != nil {
		return err
	}

	err = addRedirectFilter(netHandle, tuntap, eth)
	if err != nil {
		return err
	}

	return nil
}

// tc qdisc add dev $SRC_IFACE ingress
func addIngressQdisc(netHandle *netlink.Handle, link netlink.Link) error {
	qdisc := &netlink.Ingress{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_INGRESS,
		},
	}

	if err := netHandle.QdiscAdd(qdisc); err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

// tc filter add dev $SRC_IFACE parent ffff:
// protocol all
// u32 match u32 0 0
// action mirred egress mirror dev $DST_IFACE
func addRedirectFilter(netHandle *netlink.Handle, linkSrc, linkDest netlink.Link) error {
	filter := &netlink.U32{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: linkSrc.Attrs().Index,
			Parent:    netlink.MakeHandle(0xffff, 0),
			Protocol:  syscall.ETH_P_ALL,
		},
		Actions: []netlink.Action{
			&netlink.MirredAction{
				ActionAttrs: netlink.ActionAttrs{
					Action: netlink.TC_ACT_STOLEN,
				},
				MirredAction: netlink.TCA_EGRESS_MIRROR,
				Ifindex:      linkDest.Attrs().Index,
			},
		},
	}

	return netHandle.FilterAdd(filter)
}

// createTAPAdapter creates a new TAP device with the given name
func createTAPAdapter(netHandle *netlink.Handle, tapName string) (*netlink.Tuntap, error) {
	la := netlink.NewLinkAttrs()
	la.Name = tapName
	tuntap := &netlink.Tuntap{
		LinkAttrs: la,
		Mode:      netlink.TUNTAP_MODE_TAP,
	}

	return tuntap, addLink(netHandle, tuntap)
}

// addLink creates the given link and brings it up
func addLink(netHandle *netlink.Handle, link netlink.Link) (err error) {
	if err = netHandle.LinkAdd(link); err == nil {
		err = netHandle.LinkSetUp(link)
	}

	return
}
