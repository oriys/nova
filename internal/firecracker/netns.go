package firecracker

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
)

// SetupNetNS creates an isolated network namespace for a VM.
// Steps:
//  1. ip netns add nova-{vmID}
//  2. Create veth pair: veth-{id} (host) <-> veth-{id}-ns (netns)
//  3. Attach host end to novabr0 bridge
//  4. Move netns end into the namespace and configure IP
//  5. Create TAP device inside netns for Firecracker
//  6. Configure default route inside netns
func (m *Manager) SetupNetNS(vm *VM, gw string) error {
	nsName := "nova-" + vm.ID
	vethHost := "veth-" + vm.ID[:6]
	vethNS := "veth-" + vm.ID[:6] + "n"
	tap := "nova-" + vm.ID[:6]

	// 1. Create network namespace
	if out, err := exec.Command("ip", "netns", "add", nsName).CombinedOutput(); err != nil {
		return fmt.Errorf("create netns %s: %s: %w", nsName, out, err)
	}

	cleanup := func() {
		exec.Command("ip", "netns", "del", nsName).Run()
	}

	// 2. Create veth pair
	if out, err := exec.Command("ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethNS).CombinedOutput(); err != nil {
		cleanup()
		return fmt.Errorf("create veth pair: %s: %w", out, err)
	}

	// 3. Attach host end to bridge
	if out, err := exec.Command("ip", "link", "set", vethHost, "master", m.config.BridgeName).CombinedOutput(); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("attach veth to bridge: %s: %w", out, err)
	}
	if out, err := exec.Command("ip", "link", "set", vethHost, "up").CombinedOutput(); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("bring up veth host: %s: %w", out, err)
	}

	// 4. Move netns end into namespace
	if out, err := exec.Command("ip", "link", "set", vethNS, "netns", nsName).CombinedOutput(); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("move veth to netns: %s: %w", out, err)
	}

	// Configure IP on veth inside netns
	nsExec := func(args ...string) ([]byte, error) {
		cmdArgs := append([]string{"netns", "exec", nsName}, args...)
		return exec.Command("ip", cmdArgs...).CombinedOutput()
	}

	if out, err := nsExec("ip", "addr", "add", vm.GuestIP+"/24", "dev", vethNS); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("add ip to veth in netns: %s: %w", out, err)
	}
	if out, err := nsExec("ip", "link", "set", vethNS, "up"); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("bring up veth in netns: %s: %w", out, err)
	}
	if out, err := nsExec("ip", "link", "set", "lo", "up"); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("bring up lo in netns: %s: %w", out, err)
	}

	// 5. Create TAP device inside netns for Firecracker
	if out, err := nsExec("ip", "tuntap", "add", tap, "mode", "tap"); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("create tap in netns: %s: %w", out, err)
	}
	if out, err := nsExec("ip", "link", "set", tap, "up"); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("bring up tap in netns: %s: %w", out, err)
	}

	// 6. Default route pointing to bridge gateway
	if out, err := nsExec("ip", "route", "add", "default", "via", gw); err != nil {
		exec.Command("ip", "link", "del", vethHost).Run()
		cleanup()
		return fmt.Errorf("add default route in netns: %s: %w", out, err)
	}

	vm.NetNS = nsName
	vm.TapDevice = tap
	return nil
}

// ApplyEgressRules applies iptables egress rules inside a network namespace.
func (m *Manager) ApplyEgressRules(nsName string, policy *domain.NetworkPolicy) error {
	if policy == nil {
		return nil
	}

	iptables := func(args ...string) error {
		cmdArgs := append([]string{"netns", "exec", nsName, "iptables"}, args...)
		out, err := exec.Command("ip", cmdArgs...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("iptables %s: %s: %w", strings.Join(args, " "), out, err)
		}
		return nil
	}

	// Block all IPv6 traffic to prevent leakage
	ip6tables := func(args ...string) error {
		cmdArgs := append([]string{"netns", "exec", nsName, "ip6tables"}, args...)
		out, err := exec.Command("ip", cmdArgs...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ip6tables %s: %s: %w", strings.Join(args, " "), out, err)
		}
		return nil
	}
	_ = ip6tables("-P", "INPUT", "DROP")
	_ = ip6tables("-P", "OUTPUT", "DROP")
	_ = ip6tables("-P", "FORWARD", "DROP")

	gwIP := m.bridgeGatewayIP()

	switch policy.IsolationMode {
	case "strict":
		// Default policy: DROP all output
		if err := iptables("-P", "OUTPUT", "DROP"); err != nil {
			return err
		}
		// Allow loopback
		if err := iptables("-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"); err != nil {
			return err
		}
		// Allow established connections
		if err := iptables("-A", "OUTPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
			return err
		}
		// Allow DNS only to bridge gateway (prevent DNS exfiltration)
		if err := iptables("-A", "OUTPUT", "-p", "udp", "-d", gwIP, "--dport", "53", "-j", "ACCEPT"); err != nil {
			return err
		}
		if err := iptables("-A", "OUTPUT", "-p", "tcp", "-d", gwIP, "--dport", "53", "-j", "ACCEPT"); err != nil {
			return err
		}
		// Apply explicit allow rules
		for _, rule := range policy.EgressRules {
			if err := applyEgressRule(iptables, rule); err != nil {
				return err
			}
		}

	case "egress-only":
		if policy.DenyExternalAccess {
			// Allow RFC1918 only, drop everything else
			if err := iptables("-A", "OUTPUT", "-d", "10.0.0.0/8", "-j", "ACCEPT"); err != nil {
				return err
			}
			if err := iptables("-A", "OUTPUT", "-d", "172.16.0.0/12", "-j", "ACCEPT"); err != nil {
				return err
			}
			if err := iptables("-A", "OUTPUT", "-d", "192.168.0.0/16", "-j", "ACCEPT"); err != nil {
				return err
			}
			if err := iptables("-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"); err != nil {
				return err
			}
			// Restrict DNS to bridge gateway only
			if err := iptables("-A", "OUTPUT", "-p", "udp", "-d", gwIP, "--dport", "53", "-j", "ACCEPT"); err != nil {
				return err
			}
			if err := iptables("-A", "OUTPUT", "-j", "DROP"); err != nil {
				return err
			}
		}
		// Apply specific egress rules if present
		for _, rule := range policy.EgressRules {
			if err := applyEgressRule(iptables, rule); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyEgressRule adds a single ACCEPT rule for the given egress target.
func applyEgressRule(iptables func(...string) error, rule domain.EgressRule) error {
	args := []string{"-A", "OUTPUT", "-d", rule.Host}
	proto := rule.Protocol
	if proto == "" {
		proto = "tcp"
	}
	args = append(args, "-p", proto)
	if rule.Port > 0 {
		args = append(args, "--dport", fmt.Sprintf("%d", rule.Port))
	}
	args = append(args, "-j", "ACCEPT")
	return iptables(args...)
}

// CleanupNetNS deletes a network namespace and its associated veth pair.
func CleanupNetNS(vmID string) {
	nsName := "nova-" + vmID
	vethHost := "veth-" + vmID[:6]

	// Delete veth (automatically removes the peer)
	exec.Command("ip", "link", "del", vethHost).Run()
	// Delete namespace
	if out, err := exec.Command("ip", "netns", "del", nsName).CombinedOutput(); err != nil {
		logging.Op().Debug("cleanup netns", "ns", nsName, "output", string(out), "error", err)
	}
}

// ApplyIngressRules applies iptables INPUT rules inside a network namespace.
// This restricts which hosts/networks can initiate connections *to* the VM,
// preventing other VMs or internal hosts from accessing the function unless
// explicitly allowed.
func (m *Manager) ApplyIngressRules(nsName string, policy *domain.NetworkPolicy) error {
	if policy == nil || len(policy.IngressRules) == 0 {
		return nil // No ingress rules → default allow
	}

	iptables := func(args ...string) error {
		cmdArgs := append([]string{"netns", "exec", nsName, "iptables"}, args...)
		out, err := exec.Command("ip", cmdArgs...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("iptables %s: %s: %w", strings.Join(args, " "), out, err)
		}
		return nil
	}

	// Allow loopback and established connections
	if err := iptables("-A", "INPUT", "-i", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := iptables("-A", "INPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}

	// Allow vsock agent traffic from the host (bridge gateway)
	gwIP := m.bridgeGatewayIP()
	if err := iptables("-A", "INPUT", "-s", gwIP, "-j", "ACCEPT"); err != nil {
		return err
	}

	// Apply explicit allow rules
	for _, rule := range policy.IngressRules {
		args := []string{"-A", "INPUT", "-s", rule.Source}
		proto := rule.Protocol
		if proto == "" {
			proto = "tcp"
		}
		args = append(args, "-p", proto)
		if rule.Port > 0 {
			args = append(args, "--dport", fmt.Sprintf("%d", rule.Port))
		}
		args = append(args, "-j", "ACCEPT")
		if err := iptables(args...); err != nil {
			return err
		}
	}

	// Drop everything else
	if err := iptables("-P", "INPUT", "DROP"); err != nil {
		return err
	}

	return nil
}

// bridgeGatewayIP returns the gateway IP for the configured bridge subnet.
// For "172.30.0.0/24" this returns "172.30.0.1".
func (m *Manager) bridgeGatewayIP() string {
	parts := strings.Split(m.config.Subnet, "/")
	if len(parts) == 0 {
		return "172.30.0.1"
	}
	ip := parts[0]
	octets := strings.Split(ip, ".")
	if len(octets) != 4 {
		return "172.30.0.1"
	}
	octets[3] = "1"
	return strings.Join(octets, ".")
}
