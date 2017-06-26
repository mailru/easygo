// +build linux darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"bytes"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPoller(t *testing.T) {
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

	desc, err := Handle(conn, ModeRead|ModeOneShot)
	if err != nil {
		t.Fatal(err)
	}

	var (
		fmu   sync.Mutex
		fired []Mode
	)
	err = poller.Start(desc, func(mode Mode) {
		fmu.Lock()
		fired = append(fired, mode)
		fmu.Unlock()

		bts := make([]byte, 16)
		n, err := conn.Read(bts)
		switch {
		case err == io.EOF:
			go func() {
				defer close(done)
				if err = poller.Stop(desc); err != nil {
					t.Fatal(err)
				}
			}()

		case err != nil:
			t.Fatal(err)

		default:
			received = append(received, bts[:n]...)
			go poller.Resume(desc)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(data); i++ {
		_, err = unix.Write(w, data[i:i+1])
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
	}

	if err = unix.Close(w); err != nil {
		t.Fatal(err)
	}

	<-done

	if count, exp := len(fired), len(data)+1; count != exp { // expect +1 for EPOLLRDHUP or EPOLLHUP
		t.Errorf("callback called %d times (%v); want %d", count, fired, exp)
		return
	}

	if m := fired[len(fired)-1]; m&ModeRead == 0 || m&ModeWrite == 0 {
		t.Errorf("last callback call was made with %s; want %s", m, ModeRead|ModeWrite)
	}
	for i, m := range fired[:len(fired)-1] {
		if m != ModeRead {
			t.Errorf("callback call #%d was made with %s; want %s", i, m, ModeRead)
		}
	}
	if !bytes.Equal(data, received) {
		t.Errorf("bytes are not equal:\ngot:  %v\nwant: %v\n", received, data)
	}
}

func socketPair() (r, w int, err error) {
	fd, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return
	}
	// SetNonblock the reader part.
	if err = unix.SetNonblock(fd[0], true); err != nil {
		return
	}

	return fd[0], fd[1], nil
}

func config(tb testing.TB) *Config {
	return &Config{
		OnError: func(err error) {
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
