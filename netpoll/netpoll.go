package netpoll

import (
	"fmt"
	"log"
)

var (
	// ErrNotFiler is returned by Handle* functions to indicate that given
	// net.Conn does not provide access to its file descriptor.
	ErrNotFiler = fmt.Errorf("could not get file descriptor")

	// ErrClosed is returned by Poller methods to indicate that instance is
	// closed and operation could not be processed.
	ErrClosed = fmt.Errorf("poller instance is closed")

	// ErrRegistered is returned by Poller Start() method to indicate that
	// connection with the same underlying file descriptor is already
	// registered inside instance.
	ErrRegistered = fmt.Errorf("file descriptor is already registered in poller instance")
)

// Mode represents netpoll configuration bit mask.
type Mode uint8

// Mode values to be passed to Handle* functions and which could be received as
// an agrument to CallbackFn.
const (
	ModeRead Mode = 1 << iota
	ModeWrite
	ModeOneShot
	ModeEdgeTriggered

	// ModeClosed is a special Mode value the receipt of which means that the
	// Poller instance is closed.
	ModeClosed
)

// String returns a string representation of Mode.
func (m Mode) String() (str string) {
	name := func(mode Mode, name string) {
		if m&mode == 0 {
			return
		}
		if str != "" {
			str += "|"
		}
		str += name
	}

	name(ModeRead, "ModeRead")
	name(ModeWrite, "ModeWrite")
	name(ModeClosed, "ModeClosed")
	name(ModeOneShot, "ModeOneShot")
	name(ModeEdgeTriggered, "ModeEdgeTriggered")

	return
}

// Poller describes an object that implements logic of polling connections for
// i/o events such as availability of read() or write() operations.
type Poller interface {
	// Start adds desc to the observation list.
	//
	// Note that if desc was configured with OneShot mode on, then poller will
	// remove it from its observation list. If you will be interested in
	// receiving events after the callback, call Resume(desc).
	//
	// Note that Resume() call directly inside desc's callback could cause
	// deadlock.
	//
	// Note that multiple calls with same desc will produce unexpected
	// behavior.
	Start(*Desc, CallbackFn) error

	// Stop removes desc from the observation list.
	//
	// Note that it does not call desc.Close().
	Stop(*Desc) error

	// Resume enables observation of desc.
	//
	// It is useful when desc was configured with OneShot mode on.
	// It should be called only after Start().
	//
	// Note that if there no need to observe desc anymore, you should call
	// Stop() to prevent memory leaks.
	Resume(*Desc) error
}

// CallbackFn is a function that will be called on kernel i/o event
// notification.
type CallbackFn func(Mode)

// Config contains options for Poller configuration.
type Config struct {
	OnError func(error)
}

func (c *Config) withDefaults() (config Config) {
	if c != nil {
		config = *c
	}
	if config.OnError == nil {
		config.OnError = defaultErrorHandler
	}
	return config
}

func defaultErrorHandler(err error) {
	log.Printf("[netpoll] error: %s", err)
}
