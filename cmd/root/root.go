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

package root

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mazzy89/containervmm/pkg/api"
	"github.com/mazzy89/containervmm/pkg/disk"
	"github.com/mazzy89/containervmm/pkg/distro"
	"github.com/mazzy89/containervmm/pkg/hypervisor"
	"github.com/mazzy89/containervmm/pkg/network"
)

const targetName = "containervmm"

var c = viper.New()

var (
	cfgGuestName            = "guest-name"
	cfgGuestMemory          = "guest-memory"
	cfgGuestCPUs            = "guest-cpus"
	cfgGuestRootDiskSize    = "guest-root-disk-size"
	cfgGuestAdditionalDisks = "guest-additional-disks"
	cfgGuestHostVolumes     = "guest-host-volumes"

	cfgFlatcarChannel  = "flatcar-channel"
	cfgFlatcarVersion  = "flatcar-version"
	cfgFlatcarIgnition = "flatcar-ignition"

	cfgDebug        = "debug"
	cfgSanityChecks = "sanity-checks"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     fmt.Sprintf("%s [options]", targetName),
	Short:   "Container Virtual Machine Manager",
	Long:    `Container Virtual Machine Manager spins up a Virtual Machine inside a container`,
	Example: fmt.Sprintf("%s --flatcar-version=2605.6.0", targetName),
	RunE: func(cmd *cobra.Command, args []string) error {
		// create Guest API object
		guest := api.Guest{
			Name:   c.GetString(cfgGuestName),
			CPUs:   c.GetString(cfgGuestCPUs),
			Memory: c.GetString(cfgGuestMemory),
		}

		kernel, initrd, err := distro.DownloadImages(c.GetString(cfgFlatcarChannel), c.GetString(cfgFlatcarVersion), c.GetBool(cfgSanityChecks))
		if err != nil {
			return fmt.Errorf("an error occurred during the download of Flatcar %s %s images: %v",
				c.GetString(cfgFlatcarChannel), c.GetString(cfgFlatcarVersion), err)
		}

		// set kernel and initrd downloaded
		guest.OS.Kernel = kernel
		guest.OS.Initrd = initrd

		// set Ignition Config
		guest.OS.IgnitionConfig = c.GetString(cfgFlatcarIgnition)

		// Setup networking inside of the container, return the available interfaces
		dhcpIfaces, err := network.SetupInterfaces(&guest)
		if err != nil {
			return fmt.Errorf("an error occured during the the setup of the network: %v", err)
		}

		// Serve DHCP requests for those interfaces
		// This function returns the available IP addresses that are being
		// served over DHCP now
		if err = network.StartDHCPServers(guest, dhcpIfaces); err != nil {
			return fmt.Errorf("an error occured during the start of the DHCP servers: %v", err)
		}

		// create rootfs and other additional volumes
		gDisks := guest.Disks
		gDisks = append(gDisks, api.Disk{
			ID:     "rootfs",
			Size:   c.GetString(cfgGuestRootDiskSize),
			IsRoot: true,
		})

		for _, gd := range c.GetStringSlice(cfgGuestAdditionalDisks) {
			id, size := parseStringSliceFlag(gd)

			gDisks = append(gDisks, api.Disk{
				ID:     id,
				Size:   size,
				IsRoot: false,
			})
		}

		if err := disk.CreateDisks(&guest); err != nil {
			return fmt.Errorf("an error occured during the creation of disks: %v", err)
		}

		for _, gv := range c.GetStringSlice(cfgGuestHostVolumes) {
			mountTag, hostPath := parseStringSliceFlag(gv)

			guest.HostVolumes = append(guest.HostVolumes, api.HostVolume{
				MountTag: mountTag,
				HostPath: hostPath,
			})
		}

		// execute QEMU
		if err = hypervisor.ExecuteQEMU(guest); err != nil {
			return fmt.Errorf("an error occured during the execution of QEMU: %v", err)
		}

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	flags := rootCmd.PersistentFlags()

	flags.Bool(cfgDebug, false, "enable debug")
	_ = c.BindPFlag(cfgDebug, flags.Lookup(cfgDebug))

	flags.String(cfgGuestName, "flatcar_production_qemu", "guest name")
	_ = c.BindPFlag(cfgGuestName, flags.Lookup(cfgGuestName))

	flags.String(cfgGuestMemory, "1024M", "guest memory")
	_ = c.BindPFlag(cfgGuestMemory, flags.Lookup(cfgGuestMemory))

	flags.String(cfgGuestCPUs, "1", "guest cpus")
	_ = c.BindPFlag(cfgGuestCPUs, flags.Lookup(cfgGuestCPUs))

	flags.String(cfgGuestRootDiskSize, "20G", "guest root disk size")
	_ = c.BindPFlag(cfgGuestRootDiskSize, flags.Lookup(cfgGuestRootDiskSize))

	flags.StringSlice(cfgGuestAdditionalDisks, []string{}, "guest additional disk to mount (i.e. \"dockerfs:20GB\")")
	_ = c.BindPFlag(cfgGuestAdditionalDisks, rootCmd.PersistentFlags().Lookup(cfgGuestAdditionalDisks))
	flags.StringSlice(cfgGuestHostVolumes, []string{}, "guest host volume (i.e. \"datashare:/usr/data\")")
	_ = c.BindPFlag(cfgGuestHostVolumes, flags.Lookup(cfgGuestHostVolumes))

	flags.String(cfgFlatcarChannel, "stable", "flatcar channel (i.e. stable, beta, alpha, edge)")
	_ = c.BindPFlag(cfgFlatcarChannel, flags.Lookup(cfgFlatcarChannel))
	flags.String(cfgFlatcarVersion, "", "flatcar version")
	_ = c.BindPFlag(cfgFlatcarVersion, flags.Lookup(cfgFlatcarVersion))
	flags.String(cfgFlatcarIgnition, "", "path of the Ignition config")
	_ = c.BindPFlag(cfgFlatcarIgnition, flags.Lookup(cfgFlatcarIgnition))

	flags.Bool(cfgSanityChecks, true, "run sanity checks (GPG verification of images)")
}

func initConfig() {
	c.SetEnvPrefix(targetName)
	replacer := strings.NewReplacer("-", "_")
	c.SetEnvKeyReplacer(replacer)

	c.AutomaticEnv() // read in environment variables that match
}

func parseStringSliceFlag(input string) (string, string) {
	s := strings.Split(input, ":")

	return s[0], s[1]
}