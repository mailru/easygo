package netpoll

import (
	"net"
	"os"
	"syscall"
)

// Filer describes an object that has ability to return os.File.
type Filer interface {
	// File returns a copy of object's file descriptor.
	File() (*os.File, error)
}

// Desc is a network connection within netpoll descriptor.
// It's methods are not goroutine safe.
type Desc struct {
	file *os.File
	mode Mode
}

// Close closes underlying resources.
func (h *Desc) Close() error {
	return h.file.Close()
}

func (h *Desc) fd() int {
	return int(h.file.Fd())
}

// Handle creates new Handle with given conn and events.
// Note that EPOLLONESHOT is always added to events.
// Returned handle could be used as argument to Start, Resume and Stop methods.
func Handle(conn net.Conn, mode Mode) (*Desc, error) {
	filer, ok := conn.(Filer)
	if !ok {
		return nil, ErrNotFiler
	}

	// Get a copy of fd.
	file, err := filer.File()
	if err != nil {
		return nil, err
	}

	// Set the file back to non blocking mode since conn.File() sets underlying
	// os.File to blocking mode. This is useful to get conn.Set{Read}Deadline
	// methods still working on source Conn.
	//
	// See https://golang.org/pkg/net/#TCPConn.File
	// See /usr/local/go/src/net/net.go: conn.File()
	if err = syscall.SetNonblock(int(file.Fd()), true); err != nil {
		return nil, os.NewSyscallError("setnonblock", err)
	}

	return &Desc{
		file: file,
		mode: mode,
	}, nil
}
