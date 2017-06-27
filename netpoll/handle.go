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
	file  *os.File
	event Event
}

// Close closes underlying resources.
func (h *Desc) Close() error {
	return h.file.Close()
}

func (h *Desc) fd() int {
	return int(h.file.Fd())
}

// Must is a helper that wraps a call to a function returning (*Desc, error).
// It panics if the error is non-nil and returns desc if not.
// It is intended for use in short Desc initializations.
func Must(desc *Desc, err error) *Desc {
	if err != nil {
		panic(err)
	}
	return desc
}

// HandleRead creates read descriptor for further use in Poller methods.
// It is the same as Handle(conn, EventRead|EventEdgeTriggered).
func HandleRead(conn net.Conn) (*Desc, error) {
	return Handle(conn, EventRead|EventEdgeTriggered)
}

// HandleWrite creates write descriptor for further use in Poller methods.
// It is the same as Handle(conn, EventWrite).
func HandleWrite(conn net.Conn) (*Desc, error) {
	return Handle(conn, EventWrite)
}

// HandleReadWrite creates read and write descriptor for further use in Poller
// methods.
// It is the same as Handle(conn, EventRead|EventWrite|EventEdgeTriggered).
func HandleReadWrite(conn net.Conn) (*Desc, error) {
	return Handle(conn, EventRead|EventWrite|EventEdgeTriggered)
}

// Handle creates new Desc with given conn and event.
// Returned descriptor could be used as argument to Start(), Resume() and
// Stop() methods of some Poller implementation.
func Handle(conn net.Conn, event Event) (*Desc, error) {
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
		file:  file,
		event: event,
	}, nil
}
