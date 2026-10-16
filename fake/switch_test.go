package fake

import (
	"errors"
	"sync"
	"testing"

	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

func TestNewSwitch(t *testing.T) {
	off := NewSwitch("Porch", device.SwitchStateOff).GetSwitchData()
	if off.Name != "Porch" || !off.HasData || off.State != device.SwitchStateOff ||
		off.Connectivity != environment.ConnectivityOnline || off.LastUpdateTime.IsZero() || off.OnlineSince.IsZero() {
		t.Errorf("off: %+v", off)
	}

	unknown := NewSwitch("Porch", device.SwitchStateUnknown).GetSwitchData()
	if unknown.HasData || unknown.State != device.SwitchStateUnknown || !unknown.LastUpdateTime.IsZero() {
		t.Errorf("unknown: %+v", unknown)
	}
}

func TestSetState_TheSwitchReportsWhatItIsInNotWhatItWasAskedFor(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateOff)

	if err := s.SetState(device.SwitchStateOn); err != nil {
		t.Fatal(err)
	}

	data := s.GetSwitchData()
	if data.State != device.SwitchStateOn || data.LastChangeTime.IsZero() {
		t.Errorf("data %+v", data)
	}

	before := data.LastChangeTime

	// The same state again isn't a change.
	_ = s.SetState(device.SwitchStateOn)

	if got := s.GetSwitchData().LastChangeTime; !got.Equal(before) {
		t.Errorf("an unchanged state moved the change time: %v, was %v", got, before)
	}

	if got := s.Commands(); len(got) != 2 || got[0] != device.SwitchStateOn || got[1] != device.SwitchStateOn {
		t.Errorf("commands %v", got)
	}
}

func TestSetState_FirstReportOfASwitchThatHadNoneIsNotAChange(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateUnknown)
	_ = s.SetState(device.SwitchStateOn)

	if data := s.GetSwitchData(); !data.HasData || data.State != device.SwitchStateOn || !data.LastChangeTime.IsZero() {
		t.Errorf("data %+v", data)
	}
}

func TestSetState_InvalidStatesAreRefusedAndNotRemembered(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateOff)

	if err := s.SetState(device.SwitchStateUnknown); !errors.Is(err, device.ErrInvalidSwitchState) {
		t.Errorf("got %v", err)
	}

	if len(s.Commands()) != 0 {
		t.Errorf("commands %v", s.Commands())
	}
}

func TestFailWith(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateOff)
	boom := errors.New("boom")

	s.FailWith(boom)

	if err := s.SetState(device.SwitchStateOn); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}

	if s.GetSwitchData().State != device.SwitchStateOff {
		t.Error("a failed command changed the state")
	}

	if len(s.Commands()) != 1 {
		t.Errorf("a command that failed should still have been sent: %v", s.Commands())
	}

	s.FailWith(nil)

	if err := s.SetState(device.SwitchStateOn); err != nil {
		t.Errorf("after: %v", err)
	}
}

func TestIgnoreCommands(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateOff)
	s.IgnoreCommands(true)

	if err := s.SetState(device.SwitchStateOn); err != nil || s.GetSwitchData().State != device.SwitchStateOff {
		t.Fatalf("err %v, state %v", err, s.GetSwitchData().State)
	}

	s.IgnoreCommands(false)
	_ = s.SetState(device.SwitchStateOn)

	if s.GetSwitchData().State != device.SwitchStateOn {
		t.Error("not on")
	}
}

func TestReport(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateOff)
	s.Report(device.SwitchStateOn)

	if s.GetSwitchData().State != device.SwitchStateOn || len(s.Commands()) != 0 {
		t.Errorf("data %+v, commands %v", s.GetSwitchData(), s.Commands())
	}
}

func TestSetOffline(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateOn)
	s.SetOffline(true)

	data := s.GetSwitchData()
	if data.Connectivity != environment.ConnectivityOffline || data.OfflineSince.IsZero() || data.State != device.SwitchStateOn {
		t.Errorf("offline: %+v", data)
	}

	if err := s.SetState(device.SwitchStateOff); err == nil {
		t.Error("an offline switch accepted a command")
	}

	s.SetOffline(false)

	if data := s.GetSwitchData(); data.Connectivity != environment.ConnectivityOnline {
		t.Errorf("online: %+v", data)
	}

	if err := s.SetState(device.SwitchStateOff); err != nil {
		t.Errorf("back online: %v", err)
	}
}

func TestSwitch_SafeToUseFromManyGoroutines(t *testing.T) {
	s := NewSwitch("Porch", device.SwitchStateOff)

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(4)

		go func() { defer wg.Done(); _ = s.SetState(device.SwitchStateOn) }()
		go func() { defer wg.Done(); _ = s.GetSwitchData() }()
		go func() { defer wg.Done(); s.Report(device.SwitchStateOff) }()
		go func() { defer wg.Done(); s.FailWith(nil); s.IgnoreCommands(false); _ = s.Commands() }()
	}

	wg.Wait()
}
