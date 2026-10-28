package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// maxAncestors caps the chain the hook reports.
const maxAncestors = 16

// ancestors is replaced in tests.
var ancestors = processAncestors

// processAncestors returns this process's ancestor pids, parent first. smithersd
// checks that the Rex block's foreground process is one of them. It reads only
// pids and parent pids from one `ps` call, never another process's environment.
func processAncestors() []int {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	return chain(out, os.Getpid())
}

// chain walks `ps -o pid=,ppid=` output from self toward pid 1.
func chain(ps []byte, self int) []int {
	parent := map[int]int{}
	sc := bufio.NewScanner(bytes.NewReader(ps))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			parent[pid] = ppid
		}
	}
	var pids []int
	for p := parent[self]; p > 1 && len(pids) < maxAncestors; p = parent[p] {
		pids = append(pids, p)
	}
	return pids
}
