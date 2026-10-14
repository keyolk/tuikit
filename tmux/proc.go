package tmux

import (
	"context"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// Proc is one process-table row.
type Proc struct {
	PID, PPID int
	Command   string
}

// Procs is a process-table snapshot keyed by PID. Take one snapshot per
// refresh and walk it as often as needed: spawning ps (or pgrep) per pane is
// what made ccx's pane scan cost ~100 ms each.
type Procs map[int]Proc

// PSSource returns `ps -e -o pid=,ppid=,command=` style output. Tests pass a
// function returning a canned table.
type PSSource func(ctx context.Context) ([]byte, error)

// ExecPS runs ps.
func ExecPS(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "ps", "-e", "-o", "pid=,ppid=,command=").Output()
}

// ReadProcs takes a snapshot from src (nil is ExecPS).
func ReadProcs(ctx context.Context, src PSSource) (Procs, error) {
	if src == nil {
		src = ExecPS
	}
	out, err := src(ctx)
	if err != nil {
		return nil, err
	}
	return ParsePS(out), nil
}

// ParsePS parses "pid ppid [command…]" lines, skipping any that do not.
func ParsePS(out []byte) Procs {
	p := Procs{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		p[pid] = Proc{PID: pid, PPID: ppid, Command: strings.Join(f[2:], " ")}
	}
	return p
}

// WalkTo follows start's parent chain up to the first PID in stop and
// returns it, or 0 when the chain reaches init or an unknown PID first
// (an orphan, or a wrapper re-parented beyond the visible tree). The walk
// is bounded, so a corrupt table with a cycle ends.
func (p Procs) WalkTo(start int, stop map[int]bool) int {
	cur := start
	for range len(p) + 4 {
		if cur <= 1 {
			return 0
		}
		if stop[cur] {
			return cur
		}
		next, ok := p[cur]
		if !ok {
			return 0
		}
		cur = next.PPID
	}
	return 0
}

// PaneOf is the pane whose process tree holds pid: its own shell, or any
// descendant behind wrappers (ccproxy, sudo, tee). ok is false outside every
// pane.
func (p Procs) PaneOf(pid int, panes []Pane) (Pane, bool) {
	stop := make(map[int]bool, len(panes))
	for _, pn := range panes {
		if pn.PID > 0 {
			stop[pn.PID] = true
		}
	}
	root := p.WalkTo(pid, stop)
	if root == 0 {
		return Pane{}, false
	}
	for _, pn := range panes {
		if pn.PID == root {
			return pn, true
		}
	}
	return Pane{}, false
}

// Descendants lists every process under root, breadth first, each once even
// when the table holds a cycle.
func (p Procs) Descendants(root int) []int {
	children := map[int][]int{}
	for pid, pr := range p {
		children[pr.PPID] = append(children[pr.PPID], pid)
	}
	for _, c := range children {
		slices.Sort(c)
	}
	var out []int
	seen := map[int]bool{root: true}
	queue := append([]int(nil), children[root]...)
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
		queue = append(queue, children[pid]...)
	}
	return out
}

// HasDescendant reports whether any process under root has a command for
// which match is true — the general form of ccx's "is claude running in
// this pane" check.
func (p Procs) HasDescendant(root int, match func(Proc) bool) bool {
	for _, pid := range p.Descendants(root) {
		if match(p[pid]) {
			return true
		}
	}
	return false
}
