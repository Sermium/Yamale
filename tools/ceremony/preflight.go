package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

// networkFinding is one reason to believe this machine is not air-gapped.
type networkFinding struct {
	what   string
	detail string
}

// checklistItem is a question only a person in the room can answer. Every one
// of them is something the machine cannot see.
type checklistItem struct {
	id       string
	question string
}

// checklist is the pre-flight the operator confirms out loud.
//
// These are not padding. Each is a way the ceremony has failed somewhere, or a
// property the network checks below are structurally unable to observe: a
// wireless card that is down now and associates in ninety seconds, a phone on
// the table, a hypervisor that will snapshot the guest's RAM to a host disk, a
// swap file that pages the phrase out of the memory this program so carefully
// zeroes.
var checklist = []checklistItem{
	{"media", "The machine booted from read-only or freshly imaged media, and will be wiped afterwards."},
	{"swap", "Swap and hibernation are disabled, so nothing can page a phrase onto a disk."},
	{"radios", "Wi-Fi, Bluetooth, WWAN and NFC are disabled in firmware, not just switched off in software."},
	{"cables", "No network cable, dock, USB tether or phone is connected, and none will be during the ceremony."},
	{"virtual", "This is bare metal, not a virtual machine. A hypervisor can write guest memory to the host's disk."},
	{"recording", "No session recorder is running: no script, tmux logging, asciinema, screen sharing or remote desktop."},
	{"cameras", "No cameras or phones are pointed at the screen, and the screen is not visible through a window or a doorway."},
	{"room", "Everyone in the room belongs here, the door is closed, and someone is watching it."},
	{"binary", "The hash of this binary printed above matches the one published for this release."},
	{"paper", "Each custodian has their own sheet, their own pen, and a tamper-evident envelope."},
}

// checklistAnswer is what the operator said, kept for the ceremony record.
type checklistAnswer struct {
	ID        string `json:"id"`
	Question  string `json:"question"`
	Confirmed bool   `json:"confirmed"`
}

// binaryHash is the SHA-256 of the running executable.
//
// Printed so the room can check the tool against a published hash before it
// generates anything. It proves nothing on its own — a substituted binary would
// print whatever hash it liked — which is why the runbook has the observer
// compute the hash with a separate tool and compare, and why this is a
// checklist item rather than a green tick.
func binaryHash() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// scanNetwork looks for evidence the machine is connected. It is evidence of
// presence only: finding nothing is not evidence of absence, and this function
// is careful never to say otherwise.
//
// Two independent signals:
//
//  1. Interfaces that are up, are not loopback, and carry a routable address.
//     A configured address on a live interface is the usual way a machine that
//     was supposed to be off the network turns out to be on it.
//
//  2. A default route, found by opening a UDP socket towards a documentation
//     address. connect(2) on a UDP socket is a pure kernel route lookup — no
//     packet is sent, nothing is resolved, and 192.0.2.1 is RFC 5737 address
//     space that must never be routed anywhere real. It is here because an
//     interface can be up with an address and still have nowhere to send
//     anything, and because a default route is the signal that actually
//     correlates with reachability.
//
// This program has no other network code, and this call sends no bytes. That
// matters more than the check does: a key-generation tool that could open a
// connection is one somebody has to take on trust.
func scanNetwork() []networkFinding {
	var findings []networkFinding

	interfaces, err := net.Interfaces()
	if err != nil {
		findings = append(findings, networkFinding{
			what:   "interfaces could not be enumerated",
			detail: err.Error(),
		})
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			// Link-local is not evidence of a network: an interface with a
			// cable in it and no DHCP server gets one, and so does a laptop
			// with a dock attached to nothing. Reporting those as "connected"
			// would train operators to acknowledge the warning every time,
			// which is how a real finding gets waved through.
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			findings = append(findings, networkFinding{
				what:   fmt.Sprintf("interface %s is up with a routable address", iface.Name),
				detail: ip.String(),
			})
		}
	}

	if conn, err := net.Dial("udp", "192.0.2.1:9"); err == nil {
		local := conn.LocalAddr().String()
		_ = conn.Close()
		findings = append(findings, networkFinding{
			what:   "the kernel has a default route",
			detail: "traffic to an off-machine address would leave via " + local,
		})
	}

	return findings
}

// unknowns is what the scan above is structurally incapable of seeing. Printed
// every time, next to the result, so nobody reads "no network detected" as a
// guarantee. A tool that implied one would be worse than a tool with no check
// at all, because the room would stop looking.
var unknowns = []string{
	"whether a radio is present but not associated — a Wi-Fi card with no address looks exactly like no card",
	"whether a cable, dock or phone is plugged in one second after this check runs",
	"whether this machine has ever been online, or will be before it is wiped",
	"whether a hypervisor, management engine or firmware agent has a path off this host",
	"whether swap, hibernation or a crash dump would write memory to a disk",
	"whether anything in the room is recording the screen",
}

// runPreflight is the standalone check. The same function runs at the start of
// every key ceremony, so this command is a rehearsal of it rather than a
// separate thing an operator might skip.
func runPreflight(args []string) error {
	flags := flag.NewFlagSet("preflight", flag.ExitOnError)
	acknowledge := flags.String("network-acknowledged", "", "reason for proceeding with a network detected; recorded verbatim")
	if err := flags.Parse(args); err != nil {
		return err
	}

	c := stdConsole()
	_, err := preflight(c, *acknowledge)
	return err
}

// preflight runs the machine checks and the checklist.
//
// scanNetwork is a separate argument in preflightWith so the refusal can be
// tested both ways. The behaviour that matters — refusing on a detected
// network, refusing on an unconfirmed checklist item — must not depend on
// whether the machine the test runs on happens to have a cable in it.
func preflight(c *console, acknowledgement string) ([]checklistAnswer, error) {
	return preflightWith(c, acknowledgement, scanNetwork())
}

// preflightWith refuses to continue unless the room has answered every item.
//
// There is no --force and no --yes. Both would be the flag an operator reaches
// for when the room is waiting and the observer has stepped out, which is the
// exact moment the checklist exists for. Acknowledging a detected network needs
// a written reason instead, and the reason goes into the ceremony record where
// somebody will read it later.
func preflightWith(c *console, acknowledgement string, findings []networkFinding) ([]checklistAnswer, error) {
	c.println("=== pre-flight ===")
	c.println()

	if hash, err := binaryHash(); err == nil {
		c.printf("This binary: sha256 %s\n", hash)
	} else {
		c.printf("This binary could not be hashed (%v). Verify it with a separate tool before continuing.\n", err)
	}
	c.println()

	if len(findings) == 0 {
		c.println("Checked: no interface is up with a routable address, and the kernel has no")
		c.println("default route. That is consistent with an air-gapped machine.")
	} else {
		c.println("This machine appears to be CONNECTED:")
		for _, f := range findings {
			c.printf("  ✗ %s — %s\n", f.what, f.detail)
		}
	}
	c.println()
	c.println("Not checked, and not checkable from here:")
	for _, u := range unknowns {
		c.printf("  · %s\n", u)
	}
	c.println()

	if len(findings) > 0 {
		if strings.TrimSpace(acknowledgement) == "" {
			return nil, fmt.Errorf(
				"refusing to generate a key on a machine with a network.\n" +
					"Disconnect it and run this again. If you have a reason to proceed anyway, it has\n" +
					"to be written down and it goes into the ceremony record:\n" +
					"  --network-acknowledged \"<reason>\"")
		}
		c.printf("Proceeding anyway, on the record: %q\n", strings.TrimSpace(acknowledgement))
		c.println()
	}

	answers := make([]checklistAnswer, 0, len(checklist))
	c.println("Confirm each of these out loud. The observer should be watching, not the screen.")
	c.println()
	for _, item := range checklist {
		ok, err := c.confirm("  " + item.question)
		if err != nil {
			return nil, err
		}
		answers = append(answers, checklistAnswer{ID: item.id, Question: item.question, Confirmed: ok})
		if !ok {
			return nil, fmt.Errorf(
				"pre-flight item %q was not confirmed: %s\n"+
					"Fix it and run this again. Nothing has been generated",
				item.id, item.question)
		}
	}

	c.println()
	c.println("Pre-flight complete.")
	c.println()
	return answers, nil
}
