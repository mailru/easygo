// +build darwin dragonfly freebsd netbsd openbsd
// +build arm 386

package netpoll

import "golang.org/x/sys/unix"

func evGet(fd int, filter KeventFilter, flags KeventFlag) unix.Kevent_t {
	return unix.Kevent_t{
		Ident:  uint32(fd),
		Filter: int16(filter),
		Flags:  uint16(flags),
	}
}
