// +build linux darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"os"
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

	desc, err := Handle(conn, Read)
	if err != nil {
		t.Fatal(err)
	}
	err = poller.Start(desc, func(Mode) {
		bts, err := ioutil.ReadAll(conn)
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
			received = append(received, bts...)
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
		time.Sleep(time.Millisecond)
	}

	if err = unix.Close(w); err != nil {
		t.Fatal(err)
	}

	<-done

	if !bytes.Equal(data, received) {
		t.Fatalf("bytes are not equal:\ngot:  %v\nwant: %v\n", received, data)
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
