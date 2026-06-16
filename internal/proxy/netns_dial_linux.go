//go:build linux

package proxy

import (
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/vishvananda/netns"
)

// dialInNamespace performs an outbound dial from inside the given network
// namespace. The listener stays in the root namespace (clients connect
// normally), but the outbound socket is created inside nsName so its packets
// egress through the WireGuard interface that lives in that namespace.
//
// It runs on a dedicated, locked OS thread: enter the target ns, dial, then
// restore the original ns. Once the connect() syscall has created the socket
// inside the namespace, the established connection keeps using it even after
// we switch the thread back to root.
//
// Note: hostname resolution may still happen in the root namespace because the
// Go resolver dispatches lookups onto separate goroutines/threads. The data
// path is correct (exits via the VPN); DNS can leak. Pass an IP literal to
// avoid this, or resolve ahead of time inside the namespace.
func dialInNamespace(nsName, network, address string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		runtime.LockOSThread()

		origns, err := netns.Get()
		if err != nil {
			runtime.UnlockOSThread()
			ch <- result{nil, fmt.Errorf("get current netns: %w", err)}
			return
		}

		targetns, err := netns.GetFromName(nsName)
		if err != nil {
			origns.Close()
			runtime.UnlockOSThread()
			ch <- result{nil, fmt.Errorf("open netns %s: %w", nsName, err)}
			return
		}

		if err := netns.Set(targetns); err != nil {
			targetns.Close()
			origns.Close()
			runtime.UnlockOSThread()
			ch <- result{nil, fmt.Errorf("enter netns %s: %w", nsName, err)}
			return
		}
		targetns.Close()

		dialer := &net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		conn, derr := dialer.Dial(network, address)

		// Restore the thread to the root namespace before releasing it.
		if rerr := netns.Set(origns); rerr != nil {
			origns.Close()
			if conn != nil {
				conn.Close()
			}
			// Do NOT unlock the thread: it is stuck in the wrong namespace,
			// so we let this goroutine exit and the runtime retire the thread.
			ch <- result{nil, fmt.Errorf("restore root netns: %w", rerr)}
			return
		}
		origns.Close()
		runtime.UnlockOSThread()
		ch <- result{conn, derr}
	}()

	r := <-ch
	return r.conn, r.err
}
