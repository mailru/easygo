// +build darwin dragonfly freebsd netbsd openbsd

package netpoll

import "fmt"

// New creates new kqueue-based Poller instance with given config.
func New(*Config) (Poller, error) {
	return nil, fmt.Errorf("kqueue-based poller is not implemented yet")
}
