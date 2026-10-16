package fakes

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	spi "github.com/avanha/pmaas-spi"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
	"github.com/avanha/pmaas-spi/events"
)

// maxHistory is how many commands a fake remembers.
const maxHistory = 20

// maxFailNext is as many commands as a fake can be told to fail in advance, to catch a count that was typed wrong.
const maxFailNext = 1000

// command is one thing PMAAS asked of the switch.
type command struct {
	at       time.Time
	state    device.SwitchState
	accepted bool
	result   string
}

// fakeSwitch is a switch with nothing behind it. It is the entity PMAAS sees, with a state of its own, and it is as
// much like a real device as is useful: it reports what state it is in rather than what it was asked for, it can go
// off the network, it can fail commands, and it can accept them without doing anything. Owned by the plugin
// goroutine.
type fakeSwitch struct {
	plugin *plugin
	name   string
	id     string

	// data is what the switch reports.
	data device.Switch

	commanded   device.SwitchState
	commandedAt time.Time
	history     []command

	ignoreCommands bool
	failNext       int

	stub *switchStub
}

var _ device.ISwitch = (*fakeSwitch)(nil)

func (s *fakeSwitch) GetSwitchData() device.Switch {
	return s.data
}

// SetState is a command from PMAAS.
func (s *fakeSwitch) SetState(state device.SwitchState) error {
	if !state.IsSettable() {
		return fmt.Errorf("unable to set %q to %s: %w", s.name, state, device.ErrInvalidSwitchState)
	}

	now := s.plugin.now()

	var failure error

	switch {
	case s.data.Connectivity == environment.ConnectivityOffline:
		failure = errors.New("the device is offline")
	case s.failNext > 0:
		s.failNext--
		failure = fmt.Errorf("the device failed the command, as it was told to (%d more to fail)", s.failNext)
	}

	if failure != nil {
		s.record(command{at: now, state: state, result: failure.Error()})
		s.plugin.persist()

		return fmt.Errorf("unable to set %q to %s: %w", s.name, state, failure)
	}

	s.commanded, s.commandedAt = state, now
	s.record(command{at: now, state: state, accepted: true})

	if !s.ignoreCommands {
		s.report(state)
	}

	s.plugin.persist()

	return nil
}

func (s *fakeSwitch) record(c command) {
	s.history = append([]command{c}, s.history...)

	if len(s.history) > maxHistory {
		s.history = s.history[:maxHistory]
	}
}

// report is the device saying what state it is in, however it came to be in it. It does what a real device's report
// does: the first establishes the state without being a change, and one that is the same as before is not news.
func (s *fakeSwitch) report(state device.SwitchState) {
	now := s.plugin.now()
	old := s.data

	s.data.LastUpdateTime = now

	switch {
	case !old.HasData:
		s.data.HasData = true
		s.data.State = state
	case old.State != state:
		s.data.State = state
		s.data.LastChangeTime = now
	default:
		return
	}

	s.plugin.broadcast(s.id, events.EntityStateChangedEvent{
		EntityEvent: s.entityEvent(),
		NewState:    s.data,
	})

	if old.HasData {
		s.plugin.broadcast(s.id, device.SwitchStateChangedEvent{
			EntityEvent: s.entityEvent(),
			NewState:    state,
			OldState:    old.State,
		})
	}
}

// setOffline takes the device off the network, or puts it back. A device that is off keeps the state it last
// reported, and says it is offline.
func (s *fakeSwitch) setOffline(offline bool) {
	if offline == (s.data.Connectivity == environment.ConnectivityOffline) {
		return
	}

	now := s.plugin.now()

	if offline {
		s.data.Connectivity, s.data.OfflineSince = environment.ConnectivityOffline, now
	} else {
		s.data.Connectivity, s.data.OnlineSince = environment.ConnectivityOnline, now
	}

	s.plugin.broadcast(s.id, events.EntityStateChangedEvent{
		EntityEvent: s.entityEvent(),
		NewState:    s.data,
	})
}

// do does something to the device from the page.
func (s *fakeSwitch) do(action common.Action, count int) error {
	switch action {
	case common.ActionReportOn, common.ActionReportOff:
		if s.data.Connectivity == environment.ConnectivityOffline {
			return common.Refuse("the device is offline, so it can't report anything")
		}

		if action == common.ActionReportOn {
			s.report(device.SwitchStateOn)
		} else {
			s.report(device.SwitchStateOff)
		}
	case common.ActionOffline:
		s.setOffline(true)
	case common.ActionOnline:
		s.setOffline(false)
	case common.ActionFailNext:
		if count < 0 || count > maxFailNext {
			return common.Refuse("the number of commands to fail must be from 0 to %d", maxFailNext)
		}

		s.failNext = count
	case common.ActionIgnoreCommands:
		s.ignoreCommands = true
	case common.ActionFollowCommands:
		s.ignoreCommands = false
	case common.ActionClearHistory:
		s.history, s.commanded, s.commandedAt = nil, device.SwitchStateUnknown, time.Time{}
	default:
		return common.Refuse("there is no action %q", action)
	}

	s.plugin.persist()

	return nil
}

func (s *fakeSwitch) entityEvent() events.EntityEvent {
	return events.EntityEvent{Id: s.id, EntityType: device.ISwitchType, Name: s.name}
}

// getStub is the thread-safe handle on the switch, made on first use. Called on the plugin goroutine, where the
// container invokes the stub factory.
func (s *fakeSwitch) getStub(container spi.IPMAASContainer) *switchStub {
	if s.stub == nil {
		s.stub = &switchStub{}
		s.stub.wrapper.Store(&spicommon.ThreadSafeEntityWrapper[device.ISwitch]{Container: container, Entity: s})
	}

	return s.stub
}

// closeStub makes every later call through the stub fail, rather than reach a switch that has been deregistered.
func (s *fakeSwitch) closeStub() {
	if s.stub != nil {
		s.stub.wrapper.Store(nil)
		s.stub = nil
	}
}

// switchStub is the switch as every other goroutine sees it: each call hops onto the plugin's.
type switchStub struct {
	wrapper atomic.Pointer[spicommon.ThreadSafeEntityWrapper[device.ISwitch]]
}

var _ device.ISwitch = (*switchStub)(nil)

func (s *switchStub) GetSwitchData() device.Switch {
	return spicommon.ThreadSafeEntityWrapperExecValueFunc(s.wrapper.Load(), func(target device.ISwitch) device.Switch {
		return target.GetSwitchData()
	})
}

func (s *switchStub) SetState(state device.SwitchState) error {
	return spicommon.ThreadSafeEntityWrapperExecValueFunc(s.wrapper.Load(), func(target device.ISwitch) error {
		return target.SetState(state)
	})
}
