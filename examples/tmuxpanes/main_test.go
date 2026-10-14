package main

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keyolk/tuikit/tmux"
)

// Every test drives a Fake: the developer runs these inside tmux, and a real
// call would act on their own panes.

func TestUpdateRunsNoTmuxAndDropsStaleListings(t *testing.T) {
	f := tmux.NewFake()
	m := newModel(tmux.Client{Runner: f})
	m, _ = m.refresh() // gen 1
	m, _ = m.refresh() // gen 2; gen 1's answer is now stale

	next, _ := m.Update(tmux.PanesMsg{Gen: 1, Panes: []tmux.Pane{{ID: "%1"}}})
	m = next.(model)
	if len(m.panes) != 0 {
		t.Fatalf("stale listing applied: %+v", m.panes)
	}
	next, _ = m.Update(tmux.PanesMsg{Gen: 2, Panes: []tmux.Pane{{ID: "%2"}}})
	m = next.(model)
	if len(m.panes) != 1 || m.panes[0].ID != "%2" {
		t.Fatalf("current listing not applied: %+v", m.panes)
	}
	if n := len(f.Calls()); n != 0 {
		t.Errorf("Update ran %d tmux calls; tmux belongs in Cmds", n)
	}
}

func TestJumpIsACmdAndErrorsShow(t *testing.T) {
	f := tmux.NewFake().On("display-message", "", errors.New("no server"))
	m := newModel(tmux.Client{Runner: f})
	m.panes = []tmux.Pane{{ID: "%4"}}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || len(f.Calls()) != 0 {
		t.Fatalf("enter must return a Cmd and run nothing yet (calls %q)", f.CallLines())
	}
	next, _ := m.Update(cmd())
	if got := next.(model).status; got == "" {
		t.Error("jump error not shown")
	}
}
