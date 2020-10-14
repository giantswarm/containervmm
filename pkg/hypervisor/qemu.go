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

package hypervisor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/kata-containers/govmm/qemu"

	"github.com/mazzy89/containervmm/pkg/api"
	"github.com/mazzy89/containervmm/pkg/logs"
	"github.com/mazzy89/containervmm/pkg/util"
)

const (
	// Path of QEMU (installed in the Docker container)
	binPath = "/usr/bin/qemu-system-x86_64"

	// QEMU QMP Socket
	qmpUDS = "/tmp/qmp-socket"

	// console socket
	consoleUDS = "console.sock"

	// shutdown timeout
	powerdownTimeout = 1 * time.Minute
)

var kernelParams = [][]string{
	{"tsc", "reliable"},
	{"no_timer_check", ""},
	{"rcupdate.rcu_expedited", "1"},
	{"i8042.direct", "1"},
	{"i8042.dumbkbd", "1"},
	{"i8042.nopnp", "1"},
	{"i8042.noaux", "1"},
	{"noreplace-smp", ""},
	{"reboot", "k"},
	// this is used to read the VM output via the UNIX socket
	{"console", "hvc0"},
	{"console", "hvc1"},
	{"cryptomgr.notests", ""},
	{"net.ifnames", "0"},
	{"pci", "lastbus=0"},
}

var kernelFlatcarParams = [][]string{
	{"flatcar.first_boot", "1"},
}

type qmpLogger struct {
	*log.Logger
}

func ExecuteQEMU(guest api.Guest) error {
	// create a context
	ctx := context.Background()

	// set the list of QEMU parameters
	qemuConfig, err := createSandbox(ctx, guest)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %v", err)
	}

	if _, err := qemu.LaunchQemu(qemuConfig, newQMPLogger()); err != nil {
		return fmt.Errorf("failed to launch QEMU instance: %v", err)
	}

	if err := watchConsole(); err != nil {
		return fmt.Errorf("failed to watch console output: %v", err)
	}

	// This channel will be closed when the instance dies.
	disconnectedCh := make(chan struct{})

	// Set up our options.
	cfg := qemu.QMPConfig{Logger: newQMPLogger()}

	// Start monitoring the qemu instance.  This functon will block until we have
	// connect to the QMP socket and received the welcome message.
	q, _, err := qemu.QMPStart(ctx, qmpUDS, cfg, disconnectedCh)
	if err != nil {
		return fmt.Errorf("failed to connect to the QMP socket: %v", err)
	}

	// This has to be the first command executed in a QMP session.
	if err := q.ExecuteQMPCapabilities(ctx); err != nil {
		return fmt.Errorf("failed to run QMP commmand: %v", err)
	}

	installSignalHandlers(ctx, q)

	// disconnectedCh is closed when the VM exits. This line blocks until this
	// event occurs.
	<-disconnectedCh

	return nil
}

func newQMPLogger() qmpLogger {
	return qmpLogger{
		logs.Logger,
	}
}

func (l qmpLogger) V(level int32) bool {
	return l.IsLevelEnabled(log.Level(level))
}

func createSandbox(ctx context.Context, guest api.Guest) (qemu.Config, error) {
	knobs := qemu.Knobs{
		NoUserConfig: true,
		NoDefaults:   true,
		NoGraphic:    true,
		Daemonize:    true,
	}

	kernel, err := kernel(guest)
	if err != nil {
		return qemu.Config{}, fmt.Errorf("failed to create kernel object: %v", err)
	}

	mem := memory(guest)

	smp, err := smp(guest)
	if err != nil {
		return qemu.Config{}, fmt.Errorf("failed to create smp object: %v", err)
	}

	devices := buildDevices(guest)

	config := qemu.Config{
		Name:       guest.Name,
		Path:       binPath,
		Ctx:        ctx,
		CPUModel:   cpuModel(),
		Machine:    machine(),
		VGA:        vga(),
		Knobs:      knobs,
		Kernel:     kernel,
		Memory:     mem,
		SMP:        smp,
		QMPSockets: qmpSockets(),
		Devices:    devices,
	}

	fwcfgs := fwcfgs(guest.OS.IgnitionConfig)
	if fwcfgs != nil {
		config.FwCfg = fwcfgs
	}

	return config, nil
}

func cpuModel() string {
	return "host,pmu=off"
}

func machine() qemu.Machine {
	defaultType := "q35"
	kvmAcceleration := "kvm"

	m := qemu.Machine{
		Type:         defaultType,
		Acceleration: kvmAcceleration,
	}

	return m
}

func vga() string {
	return "none"
}

func kernel(guest api.Guest) (qemu.Kernel, error) {
	if !util.FileExists(guest.OS.Kernel) {
		return qemu.Kernel{}, fmt.Errorf("file %s not found", guest.OS.Kernel)
	}

	if !util.FileExists(guest.OS.Initrd) {
		return qemu.Kernel{}, fmt.Errorf("file %s not found", guest.OS.Initrd)
	}

	k := qemu.Kernel{
		Path:       guest.OS.Kernel,
		InitrdPath: guest.OS.Initrd,
	}

	for i := range guest.Disks {
		d := guest.Disks[i]

		if d.IsRoot {
			diskSerial := fmt.Sprintf("/dev/disk/by-id/virtio-%s", d.ID)

			rootDisk := []string{"root", diskSerial}
			rootFlag := []string{"rootflags", "rw"}

			kernelParams = append(kernelParams, rootDisk)
			kernelParams = append(kernelParams, rootFlag)
		}
	}

	kernelParams = append(kernelParams, kernelFlatcarParams...)

	k.Params = kernelCommandLine(kernelParams)

	return k, nil
}

func kernelCommandLine(params [][]string) string {
	var line string
	var lastElemIndex int = len(params) - 1

	for i, p := range params {
		line += fmt.Sprintf("%s=%s", p[0], p[1])

		if i != lastElemIndex {
			line += " "
		}
	}

	return strconv.Quote(line)
}

func memory(guest api.Guest) qemu.Memory {
	m := qemu.Memory{Size: guest.Memory}
	return m
}

func smp(guest api.Guest) (qemu.SMP, error) {
	cpus, err := strconv.Atoi(guest.CPUs)
	if err != nil {
		return qemu.SMP{}, err
	}

	s := qemu.SMP{CPUs: uint32(cpus)}

	return s, nil
}

func fwcfgs(ignitionConfig string) []qemu.FwCfg {
	var f []qemu.FwCfg

	if ignitionConfig == "" {
		return nil
	}

	name := "opt/org.flatcar-linux/config"

	fwcfg := qemu.FwCfg{
		Name: name,
		File: ignitionConfig,
	}

	f = append(f, fwcfg)

	return f
}

func qmpSockets() []qemu.QMPSocket {
	var q []qemu.QMPSocket

	qmpSocket := qemu.QMPSocket{
		Type:   qemu.Unix,
		Name:   qmpUDS,
		Server: true,
		NoWait: true,
	}

	q = append(q, qmpSocket)

	return q
}

func buildDevices(guest api.Guest) []qemu.Device {
	var devices []qemu.Device

	// append all the network devices
	devices = appendNetworkDevices(devices, guest.NICs)

	// append all the block devices
	devices = appendBlockDevices(devices, guest.Disks)

	// append console device
	devices = appendConsoleDevice(devices)

	// add random device
	id := "rng0"
	filename := "/dev/urandom"
	rngDevice := qemu.RngDevice{
		ID:       id,
		Filename: filename,
	}

	devices = append(devices, rngDevice)

	return devices
}

func appendNetworkDevices(devices []qemu.Device, guestNICs []api.NetworkInterface) []qemu.Device {
	for i := range guestNICs {
		nd := guestNICs[i]

		device := buildNetworkDevice(nd)
		devices = append(devices, device)
	}

	return devices
}

func buildNetworkDevice(guestNIC api.NetworkInterface) qemu.NetDevice {
	return qemu.NetDevice{
		Type:       qemu.TAP,
		ID:         guestNIC.TAP,
		Driver:     qemu.VirtioNetPCI,
		IFName:     guestNIC.TAP,
		MACAddress: guestNIC.MacAddr,

		// we configure NIC - no need to use any scripts
		Script:     "no",
		DownScript: "no",
	}
}

func appendBlockDevices(devices []qemu.Device, guestDisks []api.Disk) []qemu.Device {
	blkDevice := guestDisks[0]

	device := buildRootBlockDevice(blkDevice)
	devices = append(devices, device)

	return devices
}

func buildRootBlockDevice(disk api.Disk) qemu.BlockDevice {
	// we define here because in the lib is not defined
	var RAW qemu.BlockDeviceFormat = "raw"

	blk := qemu.BlockDevice{
		Driver:    qemu.VirtioBlock,
		ID:        disk.ID,
		AIO:       qemu.Threads,
		File:      disk.File,
		Format:    RAW,
		Interface: qemu.NoInterface,
		Transport: qemu.TransportPCI,
	}

	return blk
}

func appendConsoleDevice(devices []qemu.Device) []qemu.Device {
	serial := qemu.SerialDevice{
		Driver:    qemu.VirtioSerial,
		ID:        "serial0",
	}

	devices = append(devices, serial)

	console := qemu.CharDevice{
		Driver:   qemu.Console,
		Backend:  qemu.Socket,
		DeviceID: "console0",
		ID:       "charconsole0",
		Path:     consoleUDS,
	}

	devices = append(devices, console)

	return devices
}

func watchConsole() error {
	conn, err := net.Dial("unix", consoleUDS)
	if err != nil {
		return err
	}

	go func() {
		scanner := bufio.NewScanner(conn)

		for scanner.Scan() {
			log.Infof("%s", scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			if err == io.EOF {
				log.Info("console watcher quits")
			} else {
				log.Error("Failed to read console logs")
			}
		}
	}()

	return nil
}

func installSignalHandlers(ctx context.Context, q *qemu.QMP) {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				log.Infof("Caught SIGTERM, requesting clean shutdown")

				ctxTimeout, cancel := context.WithTimeout(ctx, powerdownTimeout)
				err := q.ExecuteSystemPowerdown(ctxTimeout)
				cancel()

				if err != nil {
					log.Errorf("QEMU shutdown failed with error: %v", err)

					err = q.ExecuteQuit(ctx)
					if err != nil {
						log.Errorf("QEMU quit failed with error: %v", err)
					}
				}

				q.Shutdown()
			case s == syscall.SIGQUIT:
				log.Infof("Caught SIGQUIT, forcing shutdown")

				err := q.ExecuteStop(ctx)
				if err != nil {
					log.Errorf("QEMU stop failed with error: %v", err)
				}
			}
		}
	}()
}
