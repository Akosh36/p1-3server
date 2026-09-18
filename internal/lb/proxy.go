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

// runListener accepts connections on listener (bound to one group's VIP)
// forever, proxying each to a backend chosen from *pool (read via the
// pointer so a concurrent Sync's pool swap takes effect for the very next
// accepted connection, without restarting the listener itself).
func runListener(ctx context.Context, listener net.Listener, vipAddr string, poolPtr *atomicPoolPtr) {
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
		go handleConn(conn, poolPtr.Load())
	}
}

func handleConn(client net.Conn, pool *backendPool) {
	defer client.Close()

	backend, ok := pool.pick()
	if !ok {
		// No healthy backend — nothing meaningful to do for a raw TCP
		// proxy (no HTTP status code to return at this layer), so refuse
		// the connection immediately rather than hanging the client.
		return
	}

	upstream, err := net.DialTimeout("tcp", backend.backend.addr(), backendDialTimeout)
	if err != nil {
		slog.Warn("lbd: backend dial failed", "backend", backend.backend.addr(), "error", err)
		return
	}
	defer upstream.Close()

	backend.activeConns.Add(1)
	defer backend.activeConns.Add(-1)

	done := make(chan struct{}, 2)
	go copyAndSignal(upstream, client, done)
	go copyAndSignal(client, upstream, done)
	<-done // one direction closing is enough to tear down the whole proxy
}

func copyAndSignal(dst, src net.Conn, done chan<- struct{}) {
	io.Copy(dst, src)
	done <- struct{}{}
}

// atomicPoolPtr is a tiny wrapper so runListener can read the current pool
// without a lock on every accepted connection.
type atomicPoolPtr struct {
	v atomic.Pointer[backendPool]
}

func (p *atomicPoolPtr) Load() *backendPool      { return p.v.Load() }
func (p *atomicPoolPtr) Store(pool *backendPool) { p.v.Store(pool) }
