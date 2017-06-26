// +build linux

package netpoll

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func config(t *testing.T) *EpollConfig {
	return &EpollConfig{
		OnError: func(err error) { t.Fatal(err) },
	}
}

func TestEpollCreate(t *testing.T) {
	s, err := EpollCreate(config(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEpollAddClosed(t *testing.T) {
	s, err := EpollCreate(config(t))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.Add(42, 0, nil); err != ErrClosed {
		t.Fatalf("Add() = %s; want %s", err, ErrClosed)
	}
}

func TestEpollDel(t *testing.T) {
	ln := RunEchoServer(t)
	defer ln.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	s, err := EpollCreate(config(t))
	if err != nil {
		t.Fatal(err)
	}

	f, err := conn.(filer).File()
	if err != nil {
		t.Fatal(err)
	}

	err = s.Add(int(f.Fd()), EPOLLIN, func(events Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Del(int(f.Fd())); err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEpollWait(t *testing.T) {
	ep, err := EpollCreate(config(t))
	if err != nil {
		t.Fatal(err)
	}

	recv := &bytes.Buffer{}
	done := make(chan struct{})

	read := func(fd int) CallbackFn {
		var closed bool
		return func(evt Event) {
			var buf [1024]byte
			for {
				n, _ := unix.Read(fd, buf[:])
				if n == 0 && !closed {
					close(done)
					closed = true
				}
				if n <= 0 {
					break
				}
				recv.Write(buf[:n])
			}
		}
	}

	accept := func(fd int) CallbackFn {
		return func(evt Event) {
			if evt&EPOLLCLOSE != 0 {
				return
			}
			conn, _, err := unix.Accept(fd)
			if err != nil {
				t.Fatalf("could not accept: %s", err)
			}
			unix.SetNonblock(conn, true)
			go ep.Add(conn, EPOLLIN, read(conn))
		}
	}

	ln, err := listen(4444)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(ln)

	ep.Add(ln, unix.EPOLLIN, accept(ln))

	data := []byte("hello, epoll!")

	conn, err := net.Dial("tcp", "127.0.0.1:4444")
	if err != nil {
		t.Fatalf("could not dial: %s", err)
	}

	for i := 0; i < len(data); i++ {
		if _, err := conn.Write(data[i : i+1]); err != nil {
			t.Fatalf("could not make %d-th write (%v): %s", i, string(data[i]), err)
		}
		time.Sleep(time.Millisecond)
	}
	conn.Close()

	<-done

	if err = ep.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recv.Bytes(), data) {
		t.Errorf("bytes not equal")
	}
}

func listen(port int) (ln int, err error) {
	ln, err = unix.Socket(unix.AF_INET, unix.O_NONBLOCK|unix.SOCK_STREAM, 0)
	if err != nil {
		return
	}

	// Need for avoid receiving EADDRINUSE error.
	// Closed listener could be in TIME_WAIT state some time.
	unix.SetsockoptInt(ln, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)

	addr := &unix.SockaddrInet4{
		Port: port,
		Addr: [4]byte{0x7f, 0, 0, 1}, // 127.0.0.1
	}
	if err = unix.Bind(ln, addr); err != nil {
		return
	}
	if err = unix.Listen(ln, 4); err != nil {
		return
	}
	return
}

func RunEchoServer(tb *testing.TB) net.Listener {
	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		tb.Fatal(err)
		return nil
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "closed network") {
					// Server closed.
					return
				}
				tb.Fatal(err)
			}
			go func() {
				if _, err := io.Copy(conn, conn); err != io.EOF {
					tb.Fatal(err)
				}
			}()
		}
	}()
	return ln
}
