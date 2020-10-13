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

import "github.com/mazzy89/containervmm/pkg/api"

func BindDHCPInterfaces(guest *api.Guest, dhcpIfaces []DHCPInterface) {
	var nics []api.NetworkInterface

	for i := range dhcpIfaces {
		dhcpIface := dhcpIfaces[i]

		nics = append(nics, api.NetworkInterface{
			GatewayIP:   dhcpIface.GatewayIP,
			InterfaceIP: &dhcpIface.VMIPNet.IP,
			MacAddr:     dhcpIface.MACFilter,
			TAP:         dhcpIface.VMTAP,
		})
	}

	guest.NICs = nics
}
