// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd

package netpoll

import "fmt"

func New(*Config) (Poller, error) {
	return nil, fmt.Errorf("poller is not supported on this operating system")
}
