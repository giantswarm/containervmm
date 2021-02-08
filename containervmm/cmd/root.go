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

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/mazzy89/containervmm/pkg/api"
	"github.com/mazzy89/containervmm/pkg/disk"
	"github.com/mazzy89/containervmm/pkg/distro"
	"github.com/mazzy89/containervmm/pkg/hypervisor"
	"github.com/mazzy89/containervmm/pkg/network"

	"github.com/spf13/cobra"
)

const targetName = "containervmm"

type runOptions struct {
	guestName string

	guestMemory          string
	guestCPUs            string
	guestRootDiskSize    string
	guestAdditionalDisks []string
	guestHostVolumes     []string

	flatcarChannel string
	flatcarVersion string

	// path where the Ignition config is stored
	flatcarIgnitionConfig string

	sanityChecks bool
	debug        bool
}

var opts runOptions

func Run() *cobra.Command {
	// rootCmd represents the base command when called without any subcommands
	var rootCmd = &cobra.Command{
		Use:   targetName,
		Short: "Container Virtual Machine Manager",
		Long:  `Container Virtual Machine Manager spins up a Virtual Machine inside a container`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// create Guest API object
			guest := api.Guest{
				Name:   opts.guestName,
				CPUs:   opts.guestCPUs,
				Memory: opts.guestMemory,
			}

			kernel, initrd, err := distro.DownloadImages(opts.flatcarChannel, opts.flatcarVersion, opts.sanityChecks)
			if err != nil {
				return fmt.Errorf("an error occurred during the download of Flatcar %s %s images: %v", opts.flatcarChannel, opts.flatcarVersion, err)
			}

			// set kernel and initrd downloaded
			guest.OS.Kernel = kernel
			guest.OS.Initrd = initrd
			// set Ignition Config
			guest.OS.IgnitionConfig = opts.flatcarIgnitionConfig

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
				Size:   opts.guestRootDiskSize,
				IsRoot: true,
			})

			if len(opts.guestAdditionalDisks) > 0 {
				for _, gd := range opts.guestAdditionalDisks {
					s := strings.Split(gd, ":")
					id, size := s[0], s[1]

					gDisks = append(gDisks, api.Disk{
						ID:     id,
						Size:   size,
						IsRoot: false,
					})
				}
			}

			if err := disk.CreateDisks(&guest); err != nil {
				return fmt.Errorf("an error occured during the creation of disks: %v", err)
			}

			if len(opts.guestHostVolumes) > 0 {
				for _, gv := range opts.guestHostVolumes {
					s := strings.Split(gv, ":")
					mountTag, hostPath := s[0], s[1]

					guest.HostVolumes = append(guest.HostVolumes, api.HostVolume{
						MountTag: mountTag,
						HostPath: hostPath,
					})
				}
			}

			// execute QEMU
			if err = hypervisor.ExecuteQEMU(guest); err != nil {
				return fmt.Errorf("an error occured during the execution of QEMU: %v", err)
			}

			return nil
		},
	}

	flags := rootCmd.PersistentFlags()
	flags.BoolVarP(&opts.debug, "debug", "", false, "enable debug")
	flags.StringVarP(&opts.guestName, "guest-name", "", "flatcar_production_qemu", "uest name")
	flags.StringVarP(&opts.guestMemory, "guest-memory", "m", "1024M", "guest memory")
	flags.StringVarP(&opts.guestCPUs, "guest-cpus", "c", "1", "guest cpus")

	flags.StringVarP(&opts.guestRootDiskSize, "guest-root-disk-size", "s", "20G", "guest root disk size")
	flags.StringSliceVarP(&opts.guestAdditionalDisks, "guest-additional-disks", "d", []string{}, "comma-separated list of guest additional disks to mount (i.e. \"dockerfs:20GB,kubeletfs:100GB\")")
	flags.StringSliceVarP(&opts.guestHostVolumes, "guest-host-volumes", "", []string{}, "comma-separated list of guest host volumes (i.e. \"datashare:/usr/data\")")

	flags.StringVarP(&opts.flatcarChannel, "flatcar-channel", "", "stable", "flatcar channel")
	flags.StringVarP(&opts.flatcarVersion, "flatcar-version", "", "", "flatcar version")
	rootCmd.MarkPersistentFlagRequired("flatcar-version")
	flags.StringVarP(&opts.flatcarIgnitionConfig, "flatcar-ignition", "", "", "path of the Ignition config")

	flags.BoolVarP(&opts.sanityChecks, "sanity-checks", "", true, "run sanity checks (GPG verification of images)")

	return rootCmd
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	viper.AutomaticEnv() // read in environment variables that match
}
