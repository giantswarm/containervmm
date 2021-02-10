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
	"time"

	dhcp "github.com/krolaw/dhcp4"
	"github.com/krolaw/dhcp4/conn"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/mazzy89/containervmm/pkg/api"
)

var leaseDuration, _ = time.ParseDuration("4294967295s") // Infinite lease time

// DHCPInterface describes the NIC of container
type DHCPInterface struct {
	VMIPNet    *net.IPNet
	GatewayIP  *net.IP
	Routes     []netlink.Route
	VMTAP      string
	Bridge     string
	Hostname   string
	MACFilter  string
	dnsServers []byte
	ntpServers []byte
}

func StartDHCPServers(guest api.Guest, dhcpIfaces []DHCPInterface, dnsServers []string, ntpServers []string) error {
	// Fetch the DNS servers given to the container
	containerConfig, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return fmt.Errorf("failed to get DNS configuration: %v", err)
	}

	for i := range dhcpIfaces {
		dhcpIface := &dhcpIfaces[i]

		// Set the VM hostname to the VM ID
		dhcpIface.Hostname = guest.Name

		if len(dnsServers) > 0 {
			// add the DNS servers from the user input
			dhcpIface.SetDNSServers(dnsServers)
		} else {
			// Add the DNS servers from the container
			dhcpIface.SetDNSServers(containerConfig.Servers)
		}

		if len(ntpServers) > 0 {
			dhcpIface.SetNTPServers(ntpServers)
		}

		go func() {
			log.Infof("Starting DHCP server for interface %q (%s)\n", dhcpIface.Bridge, dhcpIface.VMIPNet.IP)

			if err := dhcpIface.StartBlockingServer(); err != nil {
				log.Errorf("%q DHCP server error: %v\n", dhcpIface.Bridge, err)
			}
		}()
	}

	return nil
}

// ServeDHCP responds to a DHCP request
func (i *DHCPInterface) ServeDHCP(p dhcp.Packet, msgType dhcp.MessageType, options dhcp.Options) dhcp.Packet {
	var respMsg dhcp.MessageType

	switch msgType {
	case dhcp.Discover:
		respMsg = dhcp.Offer
	case dhcp.Request:
		respMsg = dhcp.ACK
	}

	if respMsg != 0 {
		requestingMAC := p.CHAddr().String()

		if requestingMAC == i.MACFilter {
			opts := dhcp.Options{
				dhcp.OptionSubnetMask:       []byte(i.VMIPNet.Mask),
				dhcp.OptionRouter:           []byte(*i.GatewayIP),
				dhcp.OptionDomainNameServer: i.dnsServers,
				dhcp.OptionHostName:         []byte(i.Hostname),
			}

			if netRoutes := formClasslessRoutes(&i.Routes); netRoutes != nil {
				opts[dhcp.OptionClasslessRouteFormat] = netRoutes
			}

			if i.ntpServers != nil {
				opts[dhcp.OptionNetworkTimeProtocolServers] = i.ntpServers
			}

			optSlice := opts.SelectOrderOrAll(options[dhcp.OptionParameterRequestList])

			return dhcp.ReplyPacket(p, respMsg, *i.GatewayIP, i.VMIPNet.IP, leaseDuration, optSlice)
		}
	}

	return nil
}

// StartBlockingServer starts a blocking DHCP server on port 67
func (i *DHCPInterface) StartBlockingServer() error {
	packetConn, err := conn.NewUDP4BoundListener(i.Bridge, ":67")
	if err != nil {
		return err
	}

	return dhcp.Serve(packetConn, i)
}

// Parse the DNS servers for the DHCP server
func (i *DHCPInterface) SetDNSServers(dns []string) {
	for _, server := range dns {
		i.dnsServers = append(i.dnsServers, []byte(net.ParseIP(server).To4())...)
	}
}

// Parse the NTP servers for the DHCP server
func (i *DHCPInterface) SetNTPServers(ntp []string) {
	for _, server := range ntp {
		i.ntpServers = append(i.ntpServers, []byte(net.ParseIP(server).To4())...)
	}
}

func formClasslessRoutes(routes *[]netlink.Route) (formattedRoutes []byte) {
	// See RFC4332 for additional information
	// (https://tools.ietf.org/html/rfc3442)
	// For example:
	// 		routes:
	//				10.0.0.0/8 ,  gateway: 10.1.2.3
	//              192.168.1/24, gateway: 192.168.2.3
	//		would result in the following structure:
	//      []byte{8, 10, 10, 1, 2, 3, 24, 192, 168, 1, 192, 168, 2, 3}
	if routes == nil {
		return []byte{}
	}

	sortedRoutes := sortRoutes(*routes)
	for _, route := range sortedRoutes {
		if route.Dst == nil {
			route.Dst = &net.IPNet{
				IP:   net.IPv4(0, 0, 0, 0),
				Mask: net.CIDRMask(0, 32),
			}
		}

		ip := route.Dst.IP.To4()
		width, _ := route.Dst.Mask.Size()
		octets := 0
		if width > 0 {
			octets = (width-1)/8 + 1
		}

		newRoute := append([]byte{byte(width)}, ip[0:octets]...)
		gateway := route.Gw.To4()
		if gateway == nil {
			gateway = []byte{0, 0, 0, 0}
		}

		newRoute = append(newRoute, gateway...)

		formattedRoutes = append(formattedRoutes, newRoute...)
	}

	return
}

func sortRoutes(routes []netlink.Route) []netlink.Route {
	// Default route must come last, otherwise it may not get applied
	// because there is no route to its gateway yet
	var sortedRoutes []netlink.Route
	var defaultRoutes []netlink.Route

	for _, route := range routes {
		if route.Dst == nil {
			defaultRoutes = append(defaultRoutes, route)
			continue
		}
		sortedRoutes = append(sortedRoutes, route)
	}

	for _, defaultRoute := range defaultRoutes {
		sortedRoutes = append(sortedRoutes, defaultRoute)
	}

	return sortedRoutes
}
