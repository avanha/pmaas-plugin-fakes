package fakes

import (
	"fmt"

	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
)

// SwitchHandle is the assembly's handle on a fake switch. It is a device.ISwitch, so it goes wherever a real switch
// does, and it works from the moment the plugin has started. Before then, and after the plugin stops, it answers as a
// switch that has not reported anything, and refuses commands, which is how a switch that is not there yet looks.
type SwitchHandle struct {
	name string
	stub spicommon.StubHolder[device.ISwitch]
}

var _ device.ISwitch = (*SwitchHandle)(nil)

// Name is the switch's name.
func (h *SwitchHandle) Name() string {
	return h.name
}

func (h *SwitchHandle) GetSwitchData() device.Switch {
	stub, err := h.stub.Stub()
	if err != nil {
		return device.Switch{Name: h.name}
	}

	return stub.GetSwitchData()
}

func (h *SwitchHandle) SetState(state device.SwitchState) error {
	stub, err := h.stub.Stub()
	if err != nil {
		return fmt.Errorf("unable to set the fake switch %q to %s: %w", h.name, state, err)
	}

	return stub.SetState(state)
}
