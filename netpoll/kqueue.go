// +build darwin dragonfly freebsd netbsd openbsd

package netpoll

import "fmt"

func New(*Config) (Poller, error) {
	return nil, fmt.Errorf("kqueue-based poller is not implemented yet")
}
