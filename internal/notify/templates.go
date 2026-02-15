package notify

import (
	"fmt"
	"strings"
)

// PeerOfflineAlert formats an email body for a peer-offline alert.
func PeerOfflineAlert(peerName, networkName string, offlineSince string) string {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString(fmt.Sprintf("<h2>Peer Offline Alert</h2>"))
	sb.WriteString(fmt.Sprintf("<p>Peer <strong>%s</strong> on network <strong>%s</strong> has been offline since %s.</p>",
		peerName, networkName, offlineSince))
	sb.WriteString("<p>This is an automated notification from wgpilot.</p>")
	sb.WriteString("</body></html>")
	return sb.String()
}
