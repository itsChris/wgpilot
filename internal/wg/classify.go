package wg

import "strings"

// ClassifyNetlinkError translates cryptic netlink/WireGuard errors into
// actionable hints for debugging.
func ClassifyNetlinkError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "operation not permitted"):
		return "missing CAP_NET_ADMIN capability — check systemd unit AmbientCapabilities"
	case strings.Contains(msg, "file exists"):
		return "interface already exists — check for orphaned WG interfaces with 'ip link show'"
	case strings.Contains(msg, "no such device"):
		return "wireguard kernel module not loaded — run 'modprobe wireguard'"
	case strings.Contains(msg, "address already in use"):
		return "listen port already bound — check with 'ss -ulnp | grep <port>'"
	case strings.Contains(msg, "no buffer space available"):
		return "too many network interfaces — check 'ip link | wc -l'"
	case strings.Contains(msg, "invalid argument"):
		return "invalid configuration — check key format and allowed IPs syntax"
	case strings.Contains(msg, "no such file or directory"):
		return "wireguard kernel module not loaded — run 'modprobe wireguard' or check kernel version >= 5.6"
	case strings.Contains(msg, "permission denied"):
		return "permission denied — ensure the process has CAP_NET_ADMIN"
	case strings.Contains(msg, "device or resource busy"):
		return "interface is busy — another process may be using it"
	default:
		return "unknown netlink error — check 'dmesg | tail -20' for kernel messages"
	}
}
