package main

// Is this the physical computer it appears to be?
//
// It matters more here than almost anywhere. Everything key.go does about memory
// — zeroing every buffer it owns, refusing to write a phrase to a file — assumes
// the memory belongs to this machine. Under a hypervisor it does not: the guest's
// RAM is a file on the host, and an operator with console access can snapshot it,
// live-migrate it, or take a core dump, at any point, without the guest being
// able to observe that it happened. A cloud VM is that plus somebody else's
// employees.
//
// So this is best effort, and the honesty about which parts are best effort is
// the useful part. Two grades:
//
//	strong — a DMI string naming a hypervisor or a cloud vendor, the CPU's
//	         hypervisor flag, a container marker. These say "this is a guest".
//	         The interface blocks on them and demands the ceremony be called a
//	         rehearsal.
//
//	weak   — a virtual network adapter, a locally-administered MAC on a live
//	         interface. These say "a hypervisor exists on or near this host",
//	         which is what a laptop with Hyper-V, WSL, Docker or VirtualBox
//	         installed looks like from inside a perfectly physical Windows
//	         install. Shown, never blocked on.
//
// The grading is not fussiness. preflight.go already explains why: a warning that
// fires on every run is a warning operators learn to acknowledge without reading,
// and the run where it matters is then indistinguishable from the four hundred
// where it did not. A blocking gate has to be right nearly always to keep working.
//
// Nothing here contacts a network. In particular the cloud metadata services at
// 169.254.169.254 and fd00:ec2::254 are NOT queried, even though a single GET
// would answer the question outright. On an air-gapped machine that request is
// the one packet the whole ceremony is arranged to prevent, and a tool that sent
// it under some conditions would be a tool nobody could claim sends none.

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// vmFinding is one reason to believe this machine is not bare metal.
type vmFinding struct {
	What   string `json:"what"`
	Detail string `json:"detail"`
	// Strong is the difference between "this is a guest" and "a hypervisor is
	// installed somewhere on this host". Only the first blocks the ceremony.
	Strong bool `json:"strong"`
}

// hypervisorTokens are DMI substrings that mean the firmware itself is
// synthetic.
//
// Every entry is a string no physical machine ships with. Deliberately absent:
// "Microsoft Corporation" and "Oracle Corporation", which are the system
// manufacturer of a Surface laptop and the board manufacturer of, among other
// things, real Oracle hardware. Hyper-V is caught by its product name "Virtual
// Machine" and VirtualBox by "innotek" and "VirtualBox", so nothing is lost and
// a Surface no longer trips a blocking gate.
var hypervisorTokens = []string{
	"vmware", "virtualbox", "innotek", "vbox", "qemu", "kvm", "bochs",
	"xen", "hyper-v", "virtual machine", "virtual platform", "parallels",
	"bhyve", "kubevirt", "apple virtualization", "utm", "openstack",
	"nutanix", "proxmox", "cloud hypervisor", "firecracker", "standard pc",
	"hvm domu", "sea bios",
}

// cloudTokens are DMI substrings that mean somebody else owns the host.
//
// Worse than a hypervisor in general, because the operator is a company with
// staff, a support process, and a legal obligation to respond to warrants. A
// production custodian key must not be generated on one, which is why these are
// separated in the message even though both grades block.
var cloudTokens = []string{
	"amazon ec2", "amazon", "google compute engine", "googlecloud",
	"alibaba cloud", "digitalocean", "scaleway", "hetzner", "vultr",
	"linode", "oracle cloud", "tencent cloud", "huawei cloud",
	"cloudstack", "ovhcloud", "microsoft corporation virtual",
}

// virtualNICPrefixes are MAC address OUIs assigned to hypervisor vendors.
//
// Weak by construction. Finding one means a virtual adapter exists on this host,
// which is true of every developer laptop with Docker Desktop or WSL installed
// and says nothing about whether THIS operating system is the guest. Reported
// because an operator who sees "vEthernet (Default Switch)" listed has been told
// something true about their machine, and blocked on for the same reason it would
// be wrong to: it would fire almost every time.
var virtualNICPrefixes = map[string]string{
	"00:05:69": "VMware",
	"00:0c:29": "VMware",
	"00:1c:14": "VMware",
	"00:50:56": "VMware",
	"08:00:27": "VirtualBox",
	"0a:00:27": "VirtualBox host-only",
	"00:15:5d": "Hyper-V",
	"00:16:3e": "Xen",
	"52:54:00": "QEMU/KVM",
	"00:1c:42": "Parallels",
	"00:03:ff": "Microsoft virtual",
	"02:42:ac": "Docker bridge",
}

// detectVirtualisation gathers everything that can be observed without sending a
// packet or running another program.
func detectVirtualisation() []vmFinding {
	var findings []vmFinding
	findings = append(findings, dmiFindings()...)
	findings = append(findings, cpuFindings()...)
	findings = append(findings, containerFindings()...)
	findings = append(findings, interfaceFindings()...)
	return findings
}

// dmiFindings reads the machine's own description of its hardware.
//
// This is the check that actually works. Every hypervisor writes its name into
// the SMBIOS tables it synthesises for the guest, because guest operating systems
// need it to load the right drivers — so the signal is not incidental, it is
// something the hypervisor has to provide in order to function.
func dmiFindings() []vmFinding {
	var findings []vmFinding
	for _, entry := range readDMI() {
		lowered := strings.ToLower(entry.value)
		for _, token := range cloudTokens {
			if strings.Contains(lowered, token) {
				findings = append(findings, vmFinding{
					What:   "this machine's firmware identifies a cloud provider",
					Detail: fmt.Sprintf("%s = %q", entry.source, entry.value),
					Strong: true,
				})
				break
			}
		}
		for _, token := range hypervisorTokens {
			if strings.Contains(lowered, token) {
				findings = append(findings, vmFinding{
					What:   "this machine's firmware identifies a hypervisor",
					Detail: fmt.Sprintf("%s = %q", entry.source, entry.value),
					Strong: true,
				})
				break
			}
		}
	}
	return dedupe(findings)
}

// cpuFindings reads the flags the processor reports to the kernel.
//
// The hypervisor bit is set by the CPU when it is running under one, and Linux
// surfaces it in /proc/cpuinfo. Absent on Windows and macOS, where the files
// below simply do not exist and this contributes nothing — which is why the DMI
// path is the one that has a per-platform implementation.
func cpuFindings() []vmFinding {
	var findings []vmFinding

	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "flags") {
				continue
			}
			// Space-padded so "hypervisor" is matched as a flag rather than as
			// part of a longer one.
			if strings.Contains(" "+line+" ", " hypervisor ") {
				findings = append(findings, vmFinding{
					What:   "the processor reports the hypervisor flag",
					Detail: "/proc/cpuinfo lists 'hypervisor' among the CPU flags, which the hardware sets only when something is virtualising it",
					Strong: true,
				})
			}
			break
		}
	}

	if data, err := os.ReadFile("/sys/hypervisor/type"); err == nil {
		if kind := strings.TrimSpace(string(data)); kind != "" {
			findings = append(findings, vmFinding{
				What:   "the kernel names the hypervisor it is running under",
				Detail: "/sys/hypervisor/type = " + kind,
				Strong: true,
			})
		}
	}
	return findings
}

// containerFindings looks for a namespace rather than a hypervisor.
//
// A container is weaker isolation than a VM, not stronger: the host kernel is
// shared, /proc/<pid>/mem on the host reads this process's memory directly, and
// nothing in this program can prevent that. Graded strong for the same reason a
// VM is.
func containerFindings() []vmFinding {
	var findings []vmFinding
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			findings = append(findings, vmFinding{
				What:   "this is a container, not a machine",
				Detail: marker + " exists; the host kernel can read this process's memory directly",
				Strong: true,
			})
		}
	}
	return findings
}

// interfaceFindings reports virtual network adapters.
//
// Weak, always. See virtualNICPrefixes.
func interfaceFindings() []vmFinding {
	var findings []vmFinding

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		if len(iface.HardwareAddr) < 3 {
			continue
		}
		mac := iface.HardwareAddr.String()
		prefix := strings.ToLower(mac)
		if len(prefix) >= 8 {
			prefix = prefix[:8]
		}
		if vendor, ok := virtualNICPrefixes[prefix]; ok {
			findings = append(findings, vmFinding{
				What:   "a virtual network adapter is present",
				Detail: fmt.Sprintf("%s has a %s address (%s)", iface.Name, vendor, mac),
				Strong: false,
			})
			continue
		}
		// Bit 1 of the first octet is the locally-administered bit. A physical
		// adapter ships with a globally unique address from its vendor's OUI, so
		// a live interface with a locally-administered one was either configured
		// by software or synthesised — which is what AWS Nitro and GCE look
		// like. It is also what MAC randomisation on Wi-Fi looks like, hence
		// weak: a ceremony machine should have its radios off in firmware, but
		// being wrong about that must not block the ceremony.
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 && iface.HardwareAddr[0]&0x02 != 0 {
			findings = append(findings, vmFinding{
				What:   "a live interface has a locally-administered MAC address",
				Detail: fmt.Sprintf("%s = %s, which no physical adapter ships with", iface.Name, mac),
				Strong: false,
			})
		}
	}
	return findings
}

func dedupe(findings []vmFinding) []vmFinding {
	seen := map[string]bool{}
	out := make([]vmFinding, 0, len(findings))
	for _, finding := range findings {
		key := finding.What + "|" + finding.Detail
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, finding)
	}
	return out
}

// isVirtual reports whether the ceremony must be acknowledged as a rehearsal.
func isVirtual(findings []vmFinding) bool {
	for _, finding := range findings {
		if finding.Strong {
			return true
		}
	}
	return false
}

// dmiEntry is one field of the machine's hardware description, with the source
// named so an operator can go and read the same value themselves.
type dmiEntry struct {
	source string
	value  string
}

// vmUnknowns is what no amount of code here can settle. Shown alongside the
// findings, in the same spirit as preflight.go's list: a machine that reports
// nothing is a machine that reported nothing, and the room needs to hear the
// difference between that and a guarantee.
var vmUnknowns = []string{
	"whether the firmware has been configured to hide the hypervisor — every string this reads is one the host can choose",
	"whether a management engine, BMC or firmware agent can read this memory without the operating system's involvement",
	"whether this machine's disk is encrypted at rest by somebody other than you",
	"whether a cloud metadata service is reachable — this tool deliberately does not ask, because that request is the one packet an air-gapped ceremony exists to prevent",
}
