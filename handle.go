package fakes

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/avanha/pmaas-spi/alert"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

// SwitchHandle is the assembly's handle on a fake switch. It is a device.ISwitch, so it goes wherever a real switch
// does, and it works from the moment the plugin has started. Before then, and after the plugin stops, it answers as a
// switch that has not reported anything, and refuses commands, which is how a switch that is not there yet looks.
type SwitchHandle struct {
	name    string
	stub    spicommon.StubHolder[device.ISwitch]
	offline alert.OfflineAlert
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

// AlertWhenOffline sets whether, and how, there is an alert when the switch goes offline (see alert.WhenOffline). There is none
// unless it is set. Set it before the plugin starts.
func (h *SwitchHandle) AlertWhenOffline(offline alert.OfflineAlert) *SwitchHandle {
	h.offline = offline

	return h
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

// DoorSensorHandle is the assembly's handle on a fake door sensor. It is a device.IDoorSensor, which is what a real door
// sensor is, so it goes wherever one does, and it works from the moment the plugin has started. Before then, and after
// the plugin stops, it answers as a sensor that has not reported anything.
type DoorSensorHandle struct {
	name    string
	stub    spicommon.StubHolder[device.IDoorSensor]
	offline alert.OfflineAlert
}

var _ device.IDoorSensor = (*DoorSensorHandle)(nil)

// Name is the sensor's name.
func (h *DoorSensorHandle) Name() string {
	return h.name
}

func (h *DoorSensorHandle) GetDoorSensorData() device.DoorSensor {
	stub, err := h.stub.Stub()
	if err != nil {
		return device.DoorSensor{Name: h.name}
	}

	return stub.GetDoorSensorData()
}

// AlertWhenOffline sets whether, and how, there is an alert when the sensor goes offline (see alert.WhenOffline). There is
// none unless it is set. Set it before the plugin starts.
func (h *DoorSensorHandle) AlertWhenOffline(offline alert.OfflineAlert) *DoorSensorHandle {
	h.offline = offline

	return h
}

// HostHandle is the assembly's handle on a fake host. It says whether the host answers, which it does until the page takes it
// down, and it can be given to netmon in place of pinging a real host (see PluginConfig.AddHost). It can be asked from
// any goroutine, and before the plugin starts and after it stops it answers as it last did.
type HostHandle struct {
	name string
	down atomic.Bool
}

// Name is the host's name.
func (h *HostHandle) Name() string {
	return h.name
}

// Reachable is whether the host answers.
func (h *HostHandle) Reachable(context.Context) bool {
	return !h.down.Load()
}
