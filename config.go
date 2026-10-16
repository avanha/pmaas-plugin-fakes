package fakes

import (
	"fmt"
	"strings"
)

// PluginConfig configures the plugin.
type PluginConfig struct {
	switches     []*SwitchHandle
	thermometers []*ThermometerHandle
	doorSensors  []*DoorSensorHandle
	hosts        []*HostHandle
	names        map[string]bool
}

func NewPluginConfig() PluginConfig {
	return PluginConfig{names: make(map[string]bool)}
}

// claim reserves a name for a fake. A name is one fake's, whatever kind it is, since the page and its controls go by
// name. It panics if the name is empty or has been used, like regexp.MustCompile does for a pattern that can't be: fakes
// are written in the program, and a mistake in one should stop the program starting.
func (c *PluginConfig) claim(kind string, name string) {
	if strings.TrimSpace(name) == "" {
		panic(fmt.Sprintf("fakes: a %s needs a name", kind))
	}

	if c.names == nil {
		c.names = make(map[string]bool)
	}

	if c.names[name] {
		panic(fmt.Sprintf("fakes: there is already a fake called %q", name))
	}

	c.names[name] = true
}

// AddSwitch adds a fake switch, which starts off, and returns the assembly's handle on it. The handle is a
// device.ISwitch that works from the moment the plugin has started, so it can be given to whatever would use a real
// switch: swap the line that makes the real one for this.
func (c *PluginConfig) AddSwitch(name string) *SwitchHandle {
	c.claim("switch", name)

	handle := &SwitchHandle{name: name}
	c.switches = append(c.switches, handle)

	return handle
}

// AddThermometer adds a fake thermometer, which starts at 72°F, and returns the assembly's handle on it. The handle is
// an environment.IWirelessThermometer, which is what the real ones are, and works from the moment the plugin has
// started. The thermometer reports regularly while it is running, as a real one does, so that a reading doesn't get
// old; the page can stop it, to see what happens when one does.
func (c *PluginConfig) AddThermometer(name string) *ThermometerHandle {
	c.claim("thermometer", name)

	handle := &ThermometerHandle{name: name}
	c.thermometers = append(c.thermometers, handle)

	return handle
}

// AddDoorSensor adds a fake door sensor, which starts closed, and returns the assembly's handle on it. The handle is a
// device.IDoorSensor, which is what the real ones are, and works from the moment the plugin has started.
func (c *PluginConfig) AddDoorSensor(name string) *DoorSensorHandle {
	c.claim("door sensor", name)

	handle := &DoorSensorHandle{name: name}
	c.doorSensors = append(c.doorSensors, handle)

	return handle
}

// AddHost adds a fake host, which starts reachable, and returns the assembly's handle on it. The handle says whether the
// host answers, and is what netmon is given in place of pinging a real one: netmon's own prober is the handle,
//
//	netmonConfig.AddHost("fake-router", "10.0.0.1").UseProber(fakeRouter)
//
// so that everything netmon does with a real host that goes down, it does with this one, when the page takes it down.
func (c *PluginConfig) AddHost(name string) *HostHandle {
	c.claim("host", name)

	handle := &HostHandle{name: name}
	c.hosts = append(c.hosts, handle)

	return handle
}
