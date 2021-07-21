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

package disk

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	log "github.com/sirupsen/logrus"

	"github.com/giantswarm/containervmm/pkg/api"
)

func CreateDisks(guest *api.Guest) error {
	for i := range guest.Disks {
		gd := guest.Disks[i]

		// set ID
		gd.File = gd.ID + ".img"
		// set XFS statically
		gd.Filesystem = api.XFS

		if err := createDiskFile(gd.File, gd.Size); err != nil {
			return fmt.Errorf("failed to create the disk file %s: %v", gd.File, err)
		}

		if err := runMkfs(gd.Filesystem, gd.File); err != nil {
			return fmt.Errorf("failed to exec mkfs command: %v", err)
		}

		log.Infof("Created block disk %s with size %s", gd.ID, gd.Size)

		guest.Disks[i] = gd
	}

	return nil
}

func runMkfs(filesystem api.FsType, block string) error {
	command := "mkfs." + string(filesystem)

	cmd := exec.Command(command, block)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log.Errorf("Unable to run %s command: %v", command, err)
		errStr := stderr.String()

		return fmt.Errorf("%s", errStr)
	}

	return nil
}

func createDiskFile(filename string, size string) error {
	if _, err := os.OpenFile(filename, os.O_CREATE, 0644); err != nil {
		return fmt.Errorf("failed to create file %s: %v", filename, err)
	}

	sizeVal, err := formatSize(size)
	if err != nil {
		return fmt.Errorf("failed to format the disk size: %v", err)
	}

	if err := os.Truncate(filename, sizeVal); err != nil {
		return fmt.Errorf("failed to trucate the file %s: %v", filename, err)
	}

	return nil
}
