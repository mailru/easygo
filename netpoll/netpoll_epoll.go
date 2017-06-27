// +build linux

package netpoll

// New creates new epoll-based Poller instance with given config.
func New(c *Config) (Poller, error) {
	cfg := c.withDefaults()

	epoll, err := EpollCreate(&EpollConfig{
		OnError: cfg.OnError,
	})
	if err != nil {
		return nil, err
	}

	return epoller{epoll}, nil
}

// epoller implements Poller interface.
type epoller struct {
	*Epoll
}

// Start implements Poller.Start() method.
func (ep epoller) Start(desc *Desc, cb CallbackFn) error {
	return ep.Add(desc.fd(), toEpollEvent(desc.event),
		func(ep EpollEvent) {
			var event Event

			if ep&EPOLLHUP != 0 {
				event |= EventHup
			}
			if ep&EPOLLRDHUP != 0 {
				event |= EventReadHup
			}
			if ep&EPOLLIN != 0 {
				event |= EventRead
			}
			if ep&EPOLLOUT != 0 {
				event |= EventWrite
			}
			if ep&EPOLLERR != 0 {
				event |= EventErr
			}
			if ep&EPOLLCLOSED != 0 {
				event |= EventClosed
			}

			cb(event)
		},
	)
}

// Stop implements Poller.Stop() method.
func (ep epoller) Stop(desc *Desc) error {
	return ep.Del(desc.fd())
}

// Resume implements Poller.Resume() method.
func (ep epoller) Resume(desc *Desc) error {
	return ep.Mod(desc.fd(), toEpollEvent(desc.event))
}

func toEpollEvent(event Event) (ep EpollEvent) {
	if event&EventRead != 0 {
		ep |= EPOLLIN | EPOLLRDHUP
	}
	if event&EventWrite != 0 {
		ep |= EPOLLOUT
	}
	if event&EventOneShot != 0 {
		ep |= EPOLLONESHOT
	}
	if event&EventEdgeTriggered != 0 {
		ep |= EPOLLET
	}
	return ep
}
