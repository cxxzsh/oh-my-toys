package procnet

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procDir is where the kernel publishes processes and their descriptors.
const procDir = "/proc"

// tcpTables are the kernel's tables of TCP sockets, IPv4 first.
var tcpTables = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// owner finds the socket inode of the connection in the kernel's TCP tables,
// then the process that holds that inode open. Both steps read files the
// kernel exposes to every user; processes that cannot be read are skipped.
func owner(local, remote netip.AddrPort) (Process, error) {
	want := newConn(local, remote)
	inode, err := socketInode(want)
	if err != nil {
		return Process{}, err
	}
	pid, err := socketOwnerPID(inode, want)
	if err != nil {
		return Process{}, err
	}
	return linuxProcess(pid), nil
}

// socketInode returns the inode of the established socket with the ends in
// want. It fails only when no table could be read at all; a missing IPv6
// table (IPv6 disabled) is not an error.
func socketInode(want conn) (uint64, error) {
	var readErr error
	readAny := false
	for _, table := range tcpTables {
		data, err := os.ReadFile(table) //nolint:gosec // G304: table is one of the fixed /proc/net paths in tcpTables
		if err != nil {
			readErr = err
			continue
		}
		readAny = true
		for _, s := range parseProcNetTable(string(data)) {
			if s.established && want.matches(s.local, s.remote) {
				return s.inode, nil
			}
		}
	}
	if !readAny {
		return 0, fmt.Errorf("procnet: read TCP tables: %w", readErr)
	}
	return 0, notFound(want)
}

// socketOwnerPID returns the process that holds the socket inode open. The
// kernel names every socket descriptor's link "socket:[inode]".
func socketOwnerPID(inode uint64, want conn) (int, error) {
	target := "socket:[" + strconv.FormatUint(inode, 10) + "]"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return 0, fmt.Errorf("procnet: read %s: %w", procDir, err)
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if holdsLink(pid, target) {
			return pid, nil
		}
	}
	return 0, notFound(want)
}

// holdsLink reports whether one of the process's descriptors links to target.
func holdsLink(pid int, target string) bool {
	dir := filepath.Join(procDir, strconv.Itoa(pid), "fd")
	fds, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, fd := range fds {
		if link, err := os.Readlink(filepath.Join(dir, fd.Name())); err == nil && link == target {
			return true
		}
	}
	return false
}

// linuxProcess names the process from its executable, or from its comm name
// when the executable cannot be read.
func linuxProcess(pid int) Process {
	dir := filepath.Join(procDir, strconv.Itoa(pid))
	exe, err := os.Readlink(filepath.Join(dir, "exe"))
	if err == nil {
		return Process{PID: pid, Name: displayName(exe), Path: exe}
	}
	name := strconv.Itoa(pid)
	if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil { //nolint:gosec // G304: dir is /proc/<pid>, built from an integer pid
		if trimmed := strings.TrimSpace(string(comm)); trimmed != "" {
			name = trimmed
		}
	}
	return Process{PID: pid, Name: name}
}
