// +build linux darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"bytes"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPollerReadOnce(t *testing.T) {
	poller, err := New(config(t))
	if err != nil {
		t.Fatal(err)
	}

	r, w, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}

	conn, err := net.FileConn(os.NewFile(uintptr(r), "|0"))
	if err != nil {
		t.Fatal(err)
	}

	var (
		data     = []byte("hello")
		done     = make(chan struct{})
		received = make([]byte, 0, len(data))
	)

	var (
		mu     sync.Mutex
		events []Event
	)
	desc := Must(HandleReadOnce(conn))
	err = poller.Start(desc, func(event Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()

		if event&EventRead == 0 {
			return
		}

		bts := make([]byte, 128)
		n, err := conn.Read(bts)
		switch {
		case err == io.EOF:
			defer close(done)
			if err = poller.Stop(desc); err != nil {
				t.Fatalf("poller.Stop() error: %v", err)
			}

		case err != nil:
			t.Fatal(err)

		default:
			received = append(received, bts[:n]...)
			if err := poller.Resume(desc); err != nil {
				t.Fatalf("poller.Resume() error: %v", err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Write data by a single byte.
	for i := 0; i < len(data); i++ {
		_, err = unix.Write(w, data[i:i+1])
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	if err = unix.Close(w); err != nil {
		t.Fatalf("unix.Close() error: %v", err)
	}

	<-done

	if count, exp := len(events), len(data)+1; count != exp { // expect +1 for EPOLLRDHUP or EPOLLHUP
		t.Errorf("callback called %d times (%v); want %d", count, events, exp)
		return
	}

	if last, want := events[len(events)-1], EventRead|EventHup|EventReadHup; last != want {
		t.Errorf("last callback call was made with %s; want %s", last, want)
	}
	for i, m := range events[:len(events)-1] {
		if m != EventRead {
			t.Errorf("callback call #%d was made with %s; want %s", i, m, EventRead)
		}
	}
	if !bytes.Equal(data, received) {
		t.Errorf("bytes are not equal:\ngot:  %v\nwant: %v\n", received, data)
	}
}

func TestPollerWriteOnce(t *testing.T) {
	poller, err := New(config(t))
	if err != nil {
		t.Fatal(err)
	}

	r, w, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}

	sendLoWat, err := unix.GetsockoptInt(w, unix.SOL_SOCKET, unix.SO_SNDLOWAT)
	if err != nil {
		t.Fatalf("get SO_SNDLOWAT error: %v", err)
	}
	sendBuf, err := unix.GetsockoptInt(w, unix.SOL_SOCKET, unix.SO_SNDBUF)
	if err != nil {
		t.Fatalf("get SO_SNDBUF error: %v", err)
	}

	log.Printf("send buf is %d", sendBuf)
	log.Printf("send low watermark is %d", sendLoWat)

	filled, err := fillSendBuffer(w)
	if err != nil {
		t.Fatalf("fill send buffer error: %v", err)
	}
	log.Printf("filled send buffer: %d", filled)

	var (
		writeEvents = new(uint32)
		writeHup    = new(uint32)
	)

	wc, err := net.FileConn(os.NewFile(uintptr(w), "w"))
	if err != nil {
		t.Fatal(err)
	}

	desc := Must(HandleWriteOnce(wc))
	err = poller.Start(desc, func(e Event) {
		log.Printf("received event from poller: %s", e)
		atomic.AddUint32(writeEvents, 1)

		if e&EventHup != 0 {
			atomic.AddUint32(writeHup, 1)
			return
		}
		if e&EventErr != 0 {
			return
		}

		filled, err := fillSendBuffer(w)
		if err != nil {
			t.Fatalf("fill send buffer error: %v", err)
		}
		log.Printf("filled send buffer: %d", filled)

		if err := poller.Resume(desc); err != nil {
			t.Fatalf("poller.Resume() error %v", err)
		}
	})
	if err != nil {
		t.Fatalf("poller.Start(w) error: %v", err)
	}

	// Read sendLoWat-1 bytes such that next read will trigger write event.
	if n, err := unix.Read(r, make([]byte, sendLoWat-1)); err != nil {
		t.Fatalf("unix.Read() error: %v", err)
	} else {
		log.Printf("read %d (from %d: SNDLOWAT-1)", n, sendLoWat-1)
	}

	time.Sleep(time.Millisecond * 50)
	if n := atomic.LoadUint32(writeEvents); n > 0 {
		t.Fatalf("write events: %v; want 0", n)
	}

	if n, err := unix.Read(r, make([]byte, filled)); err != nil {
		t.Fatalf("empty receive buffer error: %v", err)
	} else {
		log.Printf("emptied receive buffer: %d", n)
	}
	time.Sleep(time.Millisecond * 50)
	if n := atomic.LoadUint32(writeEvents); n != 1 {
		t.Errorf("write events: %v; want 1", n)
	}

	if err := unix.Close(r); err != nil {
		t.Fatalf("unix.Close() error: %v", err)
	}
	log.Println("closed read-end of pair")
	time.Sleep(time.Millisecond * 50)
	if n := atomic.LoadUint32(writeHup); n != 1 {
		t.Errorf("hup events: %v; want 1", n)
	}
}

func emptyRecvBuffer(fd int, k int) (n int, err error) {
	for eagain := 0; eagain < 10; {
		var x int
		p := make([]byte, k+1)
		x, err = unix.Read(fd, p)
		if err != nil {
			if err == syscall.EAGAIN {
				err = nil
				eagain++
				continue
			}
			return
		}
		n += x
	}
	return
}

func fillSendBuffer(fd int) (n int, err error) {
	p := bytes.Repeat([]byte{'x'}, 1)
	for eagain := 0; eagain < 10; {
		var x int
		x, err = unix.Write(fd, p)
		if err != nil {
			if err == syscall.EAGAIN {
				err = nil
				eagain++
				continue
			}
			return
		}
		n += x
	}
	return
}

func socketPair() (r, w int, err error) {
	fd, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return
	}

	if err = unix.SetNonblock(fd[0], true); err != nil {
		return
	}
	if err = unix.SetNonblock(fd[1], true); err != nil {
		return
	}

	buf := 4096
	if err = unix.SetsockoptInt(fd[0], unix.SOL_SOCKET, unix.SO_SNDBUF, buf); err != nil {
		return
	}
	if err = unix.SetsockoptInt(fd[1], unix.SOL_SOCKET, unix.SO_SNDBUF, buf); err != nil {
		return
	}
	if err = unix.SetsockoptInt(fd[0], unix.SOL_SOCKET, unix.SO_RCVBUF, buf); err != nil {
		return
	}
	if err = unix.SetsockoptInt(fd[1], unix.SOL_SOCKET, unix.SO_RCVBUF, buf); err != nil {
		return
	}
	return fd[0], fd[1], nil
}

func config(tb testing.TB) *Config {
	return &Config{
		OnWaitError: func(err error) {
			tb.Fatal(err)
		},
	}
}

type stubConn struct{}

func (s stubConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (s stubConn) Write(b []byte) (n int, err error)  { return 0, nil }
func (s stubConn) Close() error                       { return nil }
func (s stubConn) LocalAddr() (addr net.Addr)         { return }
func (s stubConn) RemoteAddr() (addr net.Addr)        { return }
func (s stubConn) SetDeadline(t time.Time) error      { return nil }
func (s stubConn) SetReadDeadline(t time.Time) error  { return nil }
func (s stubConn) SetWriteDeadline(t time.Time) error { return nil }
