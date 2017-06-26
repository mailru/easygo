// +build linux

package netpoll

import (
	"sync"

	"golang.org/x/sys/unix"
)

type EpollEvent uint32

const (
	EPOLLIN      EpollEvent = unix.EPOLLIN
	EPOLLOUT                = unix.EPOLLOUT
	EPOLLRDHUP              = unix.EPOLLRDHUP
	EPOLLPRI                = unix.EPOLLPRI
	EPOLLERR                = unix.EPOLLERR
	EPOLLHUP                = unix.EPOLLHUP
	EPOLLET                 = unix.EPOLLET
	EPOLLONESHOT            = unix.EPOLLONESHOT

	EPOLLCLOSE = 0x20 // Is triggered on epoll close.
)

func (e EpollEvent) String() (ret string) {
	label := func(s *string, t EpollEvent, l string) {
		if e&t == 0 {
			return
		}
		if *s != "" {
			l = " | " + l
		}
		*s += l
	}
	label(&ret, EPOLLIN, "EPOLLIN")
	label(&ret, EPOLLOUT, "EPOLLOUT")
	label(&ret, EPOLLRDHUP, "EPOLLRDHUP")
	label(&ret, EPOLLPRI, "EPOLLPRI")
	label(&ret, EPOLLERR, "EPOLLERR")
	label(&ret, EPOLLHUP, "EPOLLHUP")
	label(&ret, EPOLLET, "EPOLLET")
	label(&ret, EPOLLONESHOT, "EPOLLONESHOT")
	label(&ret, EPOLLCLOSE, "EPOLLCLOSE")
	return
}

const (
	maxEventsBegin = 1024
	maxEventsStop  = 32768
)

// closeBytes used for writing to eventfd.
var closeBytes = []byte{1, 0, 0, 0, 0, 0, 0, 0}

// Epoll represents single epoll instance.
type Epoll struct {
	mu sync.RWMutex

	fd     int
	efd    int
	closed bool
	done   chan struct{}

	callbacks map[int]func(EpollEvent)
}

// EpollCreate creates new epoll instance.
// To start wait loop and be able to call {Add,Del,Mod} methods,
// you should call Wait() before.
func EpollCreate(c *Config) (*Epoll, error) {
	config := c.withDefaults()

	fd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}

	r0, _, errno := unix.Syscall(unix.SYS_EVENTFD2, 0, 0, 0)
	if errno != 0 {
		return nil, errno
	}
	efd := int(r0)

	// Set finalizer for write end of socket pair to avoid data races when
	// closing Epoll instance and EBADF errors on writing ctl bytes from callers.
	err = unix.EpollCtl(fd, unix.EPOLL_CTL_ADD, efd, &unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(efd),
	})
	if err != nil {
		unix.Close(fd)
		unix.Close(efd)
		return nil, err
	}

	ep := &Epoll{
		fd:        fd,
		efd:       efd,
		callbacks: make(map[int]func(EpollEvent)),
		done:      make(chan struct{}),
	}

	// Run wait loop.
	go ep.wait(config.OnError)

	return ep, nil
}

// Close stops wait loop and closes all underlying resources.
func (w *Epoll) Close() (err error) {
	w.mu.Lock()
	{
		if w.closed {
			w.mu.Unlock()
			return ErrClosed
		}
		w.closed = true

		if _, err = unix.Write(w.efd, closeBytes); err != nil {
			w.mu.Unlock()
			return
		}
	}
	w.mu.Unlock()

	<-w.done

	if err = unix.Close(w.efd); err != nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	for key, cb := range w.callbacks {
		if cb != nil {
			cb(EPOLLCLOSE)
		}
		delete(w.callbacks, key)
	}

	return
}

// Add adds fd to epoll set with given events.
// Callback will be called on each received event from epoll.
// Note that EPOLLCLOSE is triggered for every cb when epoll closed.
func (w *Epoll) Add(fd int, events EpollEvent, cb func(EpollEvent)) (err error) {
	ev := &unix.EpollEvent{
		Events: uint32(events),
		Fd:     int32(fd),
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrClosed
	}
	if _, has := w.callbacks[fd]; has {
		return ErrRegistered
	}
	w.callbacks[fd] = cb

	return w.sendCtl(fd, unix.EPOLL_CTL_ADD, ev)
}

// Del removes fd from epoll set.
func (w *Epoll) Del(fd int) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.callbacks, fd)

	return w.sendCtl(fd, unix.EPOLL_CTL_DEL, nil)
}

// Mod sets to listen events on fd.
func (w *Epoll) Mod(fd int, events EpollEvent) (err error) {
	ev := &unix.EpollEvent{
		Events: uint32(events),
		Fd:     int32(fd),
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.sendCtl(fd, unix.EPOLL_CTL_MOD, ev)
}

// sendCtl checks that epoll is not closed and  makes EpollCtl call.
// Read or write mutex should be held.
func (w *Epoll) sendCtl(fd int, op int, ev *unix.EpollEvent) error {
	if w.closed {
		return ErrClosed
	}
	return unix.EpollCtl(w.fd, op, fd, ev)
}

func (w *Epoll) wait(onError func(error)) {
	defer func() {
		if err := unix.Close(w.fd); err != nil {
			onError(err)
		}
		close(w.done)
	}()

	events := make([]unix.EpollEvent, maxEventsBegin)
	callbacks := make([]func(EpollEvent), 0, maxEventsBegin)

	for {
		n, err := unix.EpollWait(w.fd, events, -1)
		if err != nil {
			if temporaryErr(err) {
				continue
			}
			onError(err)
			return
		}

		callbacks = callbacks[:n]

		w.mu.RLock()
		for i := 0; i < n; i++ {
			fd := int(events[i].Fd)
			if fd == w.efd { // signal to close
				w.mu.RUnlock()
				return
			}
			callbacks[i] = w.callbacks[fd]
		}
		w.mu.RUnlock()

		for i := 0; i < n; i++ {
			if cb := callbacks[i]; cb != nil {
				cb(EpollEvent(events[i].Events))
				callbacks[i] = nil
			}
		}

		if n == len(events) && n*2 <= maxEventsStop {
			events = make([]unix.EpollEvent, n*2)
			callbacks = make([]func(EpollEvent), 0, n*2)
		}
	}
}
