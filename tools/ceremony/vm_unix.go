//go:build !windows

package main

import (
	"os"
	"strings"
)

// dmiPaths are where Linux exposes the SMBIOS strings the firmware supplied.
//
// Read as files rather than by running dmidecode, which would need root and
// would mean this program executes something. On macOS and the BSDs these paths
// do not exist and readDMI returns nothing, which is reported as "nothing found"
// rather than as "bare metal" — see the unknowns list.
var dmiPaths = []string{
	"/sys/class/dmi/id/sys_vendor",
	"/sys/class/dmi/id/product_name",
	"/sys/class/dmi/id/product_version",
	"/sys/class/dmi/id/product_family",
	"/sys/class/dmi/id/board_vendor",
	"/sys/class/dmi/id/board_name",
	"/sys/class/dmi/id/bios_vendor",
	"/sys/class/dmi/id/bios_version",
	"/sys/class/dmi/id/chassis_vendor",
}

func readDMI() []dmiEntry {
	var entries []dmiEntry
	for _, path := range dmiPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(data))
		if value == "" {
			continue
		}
		entries = append(entries, dmiEntry{source: path, value: value})
	}
	return entries
}
