//go:build !linux

package proxy

import (
	"fmt"
	"net"
)

// dialInNamespace is a no-op stub on non-Linux platforms. Network namespaces
// only exist on Linux, so this should never be reached on Windows/macOS —
// proxies on those platforms run without a namespace bound.
func dialInNamespace(nsName, network, address string) (net.Conn, error) {
	return nil, fmt.Errorf("network namespace dialing is only supported on Linux (requested ns %q)", nsName)
}
