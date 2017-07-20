// +build darwin dragonfly freebsd netbsd openbsd

package netpoll

import (
	"reflect"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

type KeventFlag int

func (flag KeventFlag) String() (str string) {
	name := func(f KeventFlag, name string) {
		if flag&f == 0 {
			return
		}
		if str != "" {
			str += "|"
		}
		str += name
	}
	name(EV_ADD, "EV_ADD")
	name(EV_ENABLE, "EV_ENABLE")
	name(EV_DISABLE, "EV_DISABLE")
	name(EV_DISPATCH, "EV_DISPATCH")
	name(EV_DELETE, "EV_DELETE")
	name(EV_RECEIPT, "EV_RECEIPT")
	name(EV_ONESHOT, "EV_ONESHOT")
	name(EV_CLEAR, "EV_CLEAR")
	name(EV_EOF, "EV_EOF")
	name(EV_ERROR, "EV_ERROR")
	return
}

const (
	// Adds the event to the kqueue. Re-adding an existing event will modify
	// the parameters of the original event, and not result in a duplicate
	// entry. Adding an event automatically enables it, unless overridden by
	// the EV_DISABLE flag.
	EV_ADD KeventFlag = unix.EV_ADD

	// Permit kevent() to return the event if it is triggered.
	EV_ENABLE = unix.EV_ENABLE

	// Disable the event so kevent() will not return it. The filter itself is
	// not disabled.
	EV_DISABLE = unix.EV_DISABLE

	// Disable the event source immediately after delivery of an event. See
	// EV_DISABLE above.
	EV_DISPATCH = unix.EV_DISPATCH

	// Removes the event from the kqueue. Events which are attached to file
	// descriptors are automatically deleted on the last close of the
	// descriptor.
	EV_DELETE = unix.EV_DELETE

	// This flag is useful for making bulk changes to a kqueue without draining
	// any pending events. When passed as input, it forces EV_ERROR to always
	// be returned. When a filter is successfully added the data field will be
	// zero.
	EV_RECEIPT = unix.EV_RECEIPT

	// Causes the event to return only the first occurrence of the filter being
	// triggered. After the user retrieves the event from the kqueue, it is
	// deleted.
	EV_ONESHOT = unix.EV_ONESHOT

	// After the event is retrieved by the user, its state is reset. This is
	// useful for filters which report state tran- sitions instead of the
	// current state. Note that some fil- ters may automatically set this flag
	// internally.
	EV_CLEAR = unix.EV_CLEAR

	// Filters may set this flag to indicate filter-specific EOF condition.
	EV_EOF = unix.EV_EOF

	// See RETURN VALUES below.
	EV_ERROR = unix.EV_ERROR
)

type KeventFilter int

func (filter KeventFilter) String() (str string) {
	switch filter {
	case EVFILT_READ:
		return "EVFILT_READ"
	case EVFILT_WRITE:
		return "EVFILT_WRITE"
	case EVFILT_AIO:
		return "EVFILT_AIO"
	case EVFILT_VNODE:
		return "EVFILT_VNODE"
	case EVFILT_PROC:
		return "EVFILT_PROC"
	case EVFILT_SIGNAL:
		return "EVFILT_SIGNAL"
	case EVFILT_TIMER:
		return "EVFILT_TIMER"
	case EVFILT_USER:
		return "EVFILT_USER"
	case _EVFILT_CLOSED:
		return "_EVFILT_CLOSED"
	default:
		return "_EVFILT_UNKNOWN"
	}
}

const (
	// Takes a descriptor as the identifier, and returns whenever there is data
	// available to read. The behav- ior of the filter is slightly different
	// depending on the descriptor type.
	EVFILT_READ KeventFilter = unix.EVFILT_READ

	// Takes a descriptor as the identifier, and returns whenever it is
	// possible to write to the descriptor. For sockets, pipes and fifos, data
	// will contain the amount of space remaining in the write buffer. The
	// filter will set EV_EOF when the reader disconnects, and for the fifo
	// case, this may be cleared by use of EV_CLEAR. Note that this filter is
	// not supported for vnodes or BPF devices. For sockets, the low water mark
	// and socket error han- dling is identical to the EVFILT_READ case.
	EVFILT_WRITE = unix.EVFILT_WRITE

	// The sigevent portion of the AIO request is filled in, with
	// sigev_notify_kqueue containing the descriptor of the kqueue that the
	// event should be attached to, sigev_notify_kevent_flags containing the
	// kevent flags which should be EV_ONESHOT, EV_CLEAR or EV_DISPATCH,
	// sigev_value containing the udata value, and sigev_notify set to
	// SIGEV_KEVENT. When the aio_*() system call is made, the event will be
	// registered with the specified kqueue, and the ident argument set to the
	// struct aiocb returned by the aio_*() system call. The filter returns
	// under the same conditions as aio_error().
	EVFILT_AIO = unix.EVFILT_AIO

	// Takes a file descriptor as the identifier and the events to watch for in
	// fflags, and returns when one or more of the requested events occurs on
	// the descriptor.
	EVFILT_VNODE = unix.EVFILT_VNODE

	// Takes the process ID to monitor as the identifier and the events to
	// watch for in fflags, and returns when the process performs one or more
	// of the requested events. If a process can normally see another process,
	// it can attach an event to it.
	EVFILT_PROC = unix.EVFILT_PROC

	// Takes the signal number to monitor as the identifier and returns when
	// the given signal is delivered to the process. This coexists with the
	// signal() and sigaction() facilities, and has a lower precedence. The
	// filter will record all attempts to deliver a sig- nal to a process, even
	// if the signal has been marked as SIG_IGN, except for the SIGCHLD signal,
	// which, if ignored, won't be recorded by the filter. Event notification
	// happens after normal signal delivery processing. data returns the number
	// of times the signal has occurred since the last call to kevent(). This
	// filter automatically sets the EV_CLEAR flag internally.
	EVFILT_SIGNAL = unix.EVFILT_SIGNAL

	// Establishes an arbitrary timer identified by ident. When adding a timer,
	// data specifies the timeout period. The timer will be periodic unless
	// EV_ONESHOT is specified. On return, data contains the number of times
	// the timeout has expired since the last call to kevent(). This filter
	// automatically sets the EV_CLEAR flag internally. There is a system wide
	// limit on the number of timers which is controlled by the
	// kern.kq_calloutmax sysctl.
	EVFILT_TIMER = unix.EVFILT_TIMER

	// Establishes a user event identified by ident which is not associated
	// with any kernel mechanism but is trig- gered by user level code.
	EVFILT_USER = unix.EVFILT_USER

	// Custom filter value signaling that kqueue instance get closed.
	_EVFILT_CLOSED = -0x7f
)

const FilterCount = 8

type Kevent struct {
	Filter KeventFilter
	Flags  KeventFlag
}

type KeventHandler func(KeventFilter, KeventFlag)

type KqueueConfig struct {
	// OnWaitError will be called from goroutine, waiting for events.
	OnWaitError func(error)
}

func (c *KqueueConfig) withDefaults() (config KqueueConfig) {
	if c != nil {
		config = *c
	}
	if config.OnWaitError == nil {
		config.OnWaitError = defaultOnWaitError
	}
	return config
}

// Kqueue represents kqueue instance.
type Kqueue struct {
	mu     sync.RWMutex
	fd     int
	cb     map[int]KeventHandler
	done   chan struct{}
	closed bool
}

// KqueueCreate creates new kqueue instance.
// It starts wait loop in a separate goroutine.
func KqueueCreate(c *KqueueConfig) (*Kqueue, error) {
	config := c.withDefaults()

	fd, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}

	kq := &Kqueue{
		fd:   fd,
		cb:   make(map[int]KeventHandler),
		done: make(chan struct{}),
	}

	go kq.wait(config.OnWaitError)

	return kq, nil
}

func (k *Kqueue) Close() error {
	//TODO
	return nil
}

func (k *Kqueue) Add(fd int, events [FilterCount]Kevent, n int, cb KeventHandler) error {
	var kevs [FilterCount]unix.Kevent_t
	for i := 0; i < n; i++ {
		kevs[i] = evGet(fd, events[i].Filter, events[i].Flags)
	}

	arr := unsafe.Pointer(&kevs)
	hdr := &reflect.SliceHeader{
		Data: uintptr(arr),
		Len:  n,
		Cap:  n,
	}
	changes := *(*[]unix.Kevent_t)(unsafe.Pointer(hdr))

	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return ErrClosed
	}
	if _, has := k.cb[fd]; has {
		return ErrRegistered
	}
	k.cb[fd] = cb

	_, err := unix.Kevent(k.fd, changes, nil, nil)

	return err
}

func (k *Kqueue) Mod(fd int, events [FilterCount]Kevent, n int) error {
	var kevs [FilterCount]unix.Kevent_t
	for i := 0; i < n; i++ {
		kevs[i] = evGet(fd, events[i].Filter, events[i].Flags)
	}

	arr := unsafe.Pointer(&kevs)
	hdr := &reflect.SliceHeader{
		Data: uintptr(arr),
		Len:  n,
		Cap:  n,
	}
	changes := *(*[]unix.Kevent_t)(unsafe.Pointer(hdr))

	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return ErrClosed
	}
	if _, has := k.cb[fd]; !has {
		return ErrNotRegistered
	}

	_, err := unix.Kevent(k.fd, changes, nil, nil)

	return err
}

// Del removes callback for fd. Note that it does not cleanups events for fd in
// kqueue. You should close fd or call Mod() with EV_DELETE flag set.
func (k *Kqueue) Del(fd int) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return ErrClosed
	}
	if _, has := k.cb[fd]; !has {
		return ErrNotRegistered
	}

	delete(k.cb, fd)

	return nil
}

func (k *Kqueue) wait(onError func(error)) {
	const (
		maxWaitEventsBegin = 1 << 10 // 1024
		maxWaitEventsStop  = 1 << 15 // 32768
	)

	defer func() {
		if err := unix.Close(k.fd); err != nil {
			onError(err)
		}
		close(k.done)
	}()

	evs := make([]unix.Kevent_t, maxWaitEventsBegin)
	cbs := make([]KeventHandler, maxWaitEventsBegin)

	for {
		n, err := unix.Kevent(k.fd, nil, evs, nil)
		if err != nil {
			if temporaryErr(err) {
				continue
			}
			onError(err)
			return
		}

		cbs = cbs[:n]
		k.mu.RLock()
		for i := 0; i < n; i++ {
			fd := int(evs[i].Ident)
			if fd == -1 { //todo
				k.mu.RUnlock()
				return
			}
			cbs[i] = k.cb[fd]
		}
		k.mu.RUnlock()

		for i, cb := range cbs {
			if cb != nil {
				cb(KeventFilter(evs[i].Filter), KeventFlag(evs[i].Flags))
				cbs[i] = nil
			}
		}

		if n == len(evs) && n*2 <= maxWaitEventsStop {
			evs = make([]unix.Kevent_t, n*2)
			cbs = make([]KeventHandler, n*2)
		}
	}
}

func evGet(fd int, filter KeventFilter, flags KeventFlag) unix.Kevent_t {
	return unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: int16(filter),
		Flags:  uint16(flags),
	}
}
