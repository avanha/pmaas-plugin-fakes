// Package fake has fakes that need no server, for unit tests: plain Go values that stand in for the things the
// plugin's entities stand in for in a running server. Use them to test code that takes a device.ISwitch.
package fake

import (
	"fmt"
	"sync"
	"time"

	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

// Switch is a device.ISwitch with nothing behind it. It does what a real switch does: it reports what state it is in,
// not what it was asked for, and it can refuse a command or ignore one. It is safe to use from several goroutines.
type Switch struct {
	mu       sync.Mutex
	data     device.Switch
	err      error
	ignore   bool
	commands []device.SwitchState
	now      func() time.Time
}

var _ device.ISwitch = (*Switch)(nil)

// NewSwitch makes a switch that is online and in the given state. SwitchStateUnknown is a switch that has not reported
// anything yet.
func NewSwitch(name string, state device.SwitchState) *Switch {
	s := &Switch{now: time.Now}
	now := s.now()

	s.data = device.Switch{
		Name:           name,
		HasData:        state != device.SwitchStateUnknown,
		State:          state,
		LastUpdateTime: now,
		Connectivity:   environment.ConnectivityOnline,
		OnlineSince:    now,
	}

	if state == device.SwitchStateUnknown {
		s.data.LastUpdateTime = time.Time{}
	}

	return s
}

func (s *Switch) GetSwitchData() device.Switch {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.data
}

// SetState is a command. Every valid command is remembered, whether or not the switch did anything about it. A switch
// that has been told to fail refuses it with that error, and one that has been told to ignore commands accepts it and
// stays as it was.
func (s *Switch) SetState(state device.SwitchState) error {
	if !state.IsSettable() {
		return fmt.Errorf("unable to set %q to %s: %w", s.data.Name, state, device.ErrInvalidSwitchState)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.commands = append(s.commands, state)

	if s.err != nil {
		return s.err
	}

	if !s.ignore {
		s.reportLocked(state)
	}

	return nil
}

// Report is the device changing state by itself, as when someone presses its button.
func (s *Switch) Report(state device.SwitchState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reportLocked(state)
}

func (s *Switch) reportLocked(state device.SwitchState) {
	now := s.now()
	s.data.LastUpdateTime = now

	switch {
	case !s.data.HasData:
		s.data.HasData = true
		s.data.State = state
	case s.data.State != state:
		s.data.State = state
		s.data.LastChangeTime = now
	}
}

// FailWith makes every later command fail with err, until it is called with nil.
func (s *Switch) FailWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.err = err
}

// IgnoreCommands makes the switch accept commands and not act on them, or, with false, act on them again.
func (s *Switch) IgnoreCommands(ignore bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ignore = ignore
}

// SetOffline takes the switch off the network, or puts it back. An offline switch refuses commands with an error that
// says so, and keeps the state it last reported.
func (s *Switch) SetOffline(offline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	if offline {
		s.data.Connectivity, s.data.OfflineSince = environment.ConnectivityOffline, now
		s.err = fmt.Errorf("%q is offline", s.data.Name)

		return
	}

	s.data.Connectivity, s.data.OnlineSince = environment.ConnectivityOnline, now
	s.err = nil
}

// Commands is every command the switch has been sent, in order.
func (s *Switch) Commands() []device.SwitchState {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]device.SwitchState(nil), s.commands...)
}
