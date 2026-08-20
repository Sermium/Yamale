//go:build windows

package main

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// dmiKeys are where Windows publishes the SMBIOS strings the firmware supplied.
//
// The registry rather than WMI, because a WMI query means launching a COM
// apartment and talking to a service, and this program's central claim is that it
// does not talk to anything. These values are the same strings Linux exposes
// under /sys/class/dmi/id — the firmware's own account of the hardware, which a
// hypervisor has to fill in for its guest's drivers to load.
var dmiKeys = []struct {
	path   string
	values []string
}{
	{
		`HARDWARE\DESCRIPTION\System\BIOS`,
		[]string{
			"SystemManufacturer", "SystemProductName", "SystemFamily",
			"SystemVersion", "BaseBoardManufacturer", "BaseBoardProduct",
			"BIOSVendor", "BIOSVersion",
		},
	},
	{
		`SYSTEM\CurrentControlSet\Control\SystemInformation`,
		[]string{"SystemManufacturer", "SystemProductName", "BIOSVendor"},
	},
}

func readDMI() []dmiEntry {
	var entries []dmiEntry
	for _, group := range dmiKeys {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, group.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		for _, name := range group.values {
			value, _, err := key.GetStringValue(name)
			if err != nil {
				// BIOSVersion is a multi-string on some machines. Read it that
				// way rather than skipping it: on Hyper-V guests it is the field
				// that says so.
				if strings2, _, err2 := key.GetStringsValue(name); err2 == nil {
					value = strings.Join(strings2, " ")
				} else {
					continue
				}
			}
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			entries = append(entries, dmiEntry{source: `HKLM\` + group.path + `\` + name, value: value})
		}
		_ = key.Close()
	}
	return entries
}
