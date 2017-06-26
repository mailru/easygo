package netpoll

import (
	"fmt"
	"log"
)

var (
	ErrNotFiler   = fmt.Errorf("could not get file descriptor")
	ErrClosed     = fmt.Errorf("poller instance is closed")
	ErrRegistered = fmt.Errorf("file descriptor is already registered in netpoll")
)

type Mode uint8

const (
	Read Mode = 1 << iota >> 1
	Write
	Close
	OneShot
	EdgeTriggered
)

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

type CallbackFn func(Mode)

type Config struct {
	OnError func(error)
}

func (c *Config) withDefaults() (config Config) {
	if c != nil {
		config = *c
	}
	if config.OnError == nil {
		config.OnError = DefaultErrorHandler
	}
	return config
}

func DefaultErrorHandler(err error) {
	log.Printf("[netpoll] error: %s", err)
}
