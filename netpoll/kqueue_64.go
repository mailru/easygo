// +build darwin dragonfly freebsd netbsd openbsd
// +build arm64 amd64

package netpoll

import "golang.org/x/sys/unix"

func evGet(fd int, filter KeventFilter, flags KeventFlag) unix.Kevent_t {
	return unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: int16(filter),
		Flags:  uint16(flags),
	}
}
