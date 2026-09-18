package lb

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"time"
)

const backendDialTimeout = 3 * time.Second

// trafficRecorder is called once per finished connection with that
// client's total bytes in each direction, so Manager can attribute usage
// per device once internal/lbsync resolves the client IP.
type trafficRecorder func(vipAddress, clientIP string, bytesUp, bytesDown int64)

// runListener accepts connections on listener (bound to one group's VIP)
// forever, proxying each to a backend chosen from *pool (read via the
// pointer so a concurrent Sync's pool swap takes effect for the very next
// accepted connection, without restarting the listener itself).
func runListener(ctx context.Context, listener net.Listener, vipAddr, vipAddress string, poolPtr *atomicPoolPtr, record trafficRecorder) {
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return // listener.Close() above caused this; not an error
			default:
				slog.Warn("lbd: accept failed", "vip", vipAddr, "error", err)
				continue
			}
		}
		go handleConn(conn, poolPtr.Load(), vipAddress, record)
	}
}

func handleConn(client net.Conn, pool *backendPool, vipAddress string, record trafficRecorder) {
	backend, ok := pool.pick()
	if !ok {
		// No healthy backend — nothing meaningful to do for a raw TCP
		// proxy (no HTTP status code to return at this layer), so refuse
		// the connection immediately rather than hanging the client.
		client.Close()
		return
	}

	upstream, err := net.DialTimeout("tcp", backend.backend.addr(), backendDialTimeout)
	if err != nil {
		slog.Warn("lbd: backend dial failed", "backend", backend.backend.addr(), "error", err)
		client.Close()
		return
	}

	backend.activeConns.Add(1)
	defer backend.activeConns.Add(-1)

	// Each direction's io.Copy is only guaranteed to unblock once BOTH
	// sockets are closed (one side finishing — e.g. the client
	// disconnecting — doesn't stop the backend from sitting there
	// mid-request forever). So: wait for the first direction to finish,
	// force-close both ends (which unblocks the second copy's blocked
	// Read/Write), then wait for that too — only then are both byte
	// counts final and safe to report.
	var bytesUp, bytesDown atomic.Int64
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(upstream, client)
		bytesUp.Store(n)
		done <- struct{}{}
	}()
	go func() {
		n, _ := io.Copy(client, upstream)
		bytesDown.Store(n)
		done <- struct{}{}
	}()
	<-done
	client.Close()
	upstream.Close()
	<-done

	if record != nil {
		clientIP, _, splitErr := net.SplitHostPort(client.RemoteAddr().String())
		if splitErr != nil {
			clientIP = client.RemoteAddr().String()
		}
		record(vipAddress, clientIP, bytesUp.Load(), bytesDown.Load())
	}
}

// atomicPoolPtr is a tiny wrapper so runListener can read the current pool
// without a lock on every accepted connection.
type atomicPoolPtr struct {
	v atomic.Pointer[backendPool]
}

func (p *atomicPoolPtr) Load() *backendPool      { return p.v.Load() }
func (p *atomicPoolPtr) Store(pool *backendPool) { p.v.Store(pool) }
