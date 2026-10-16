package fakes

import (
	"fmt"

	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
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

// ThermometerHandle is the assembly's handle on a fake thermometer. It is an environment.IWirelessThermometer, which
// is what a real thermometer is, so it goes wherever one does, and it works from the moment the plugin has started.
// Before then, and after the plugin stops, it answers as a thermometer that has not reported anything.
type ThermometerHandle struct {
	name string
	stub spicommon.StubHolder[environment.IWirelessThermometer]
}

var _ environment.IWirelessThermometer = (*ThermometerHandle)(nil)

// Name is the thermometer's name.
func (h *ThermometerHandle) Name() string {
	return h.name
}

func (h *ThermometerHandle) GetWirelessThermometerData() environment.WirelessThermometer {
	stub, err := h.stub.Stub()
	if err != nil {
		return environment.WirelessThermometer{Name: h.name}
	}

	return stub.GetWirelessThermometerData()
}
