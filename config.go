package fakes

import (
	"fmt"
	"strings"
)

// PluginConfig configures the plugin.
type PluginConfig struct {
	switches []*SwitchHandle
	names    map[string]bool
}

func NewPluginConfig() PluginConfig {
	return PluginConfig{names: make(map[string]bool)}
}

// AddSwitch adds a fake switch, which starts off, and returns the assembly's handle on it. The handle is a
// device.ISwitch that works from the moment the plugin has started, so it can be given to whatever would use a real
// switch: swap the line that makes the real one for this.
//
// It panics if the name is empty or has been used, like regexp.MustCompile does for a pattern that can't be: fakes are
// written in the program, and a mistake in one should stop the program starting.
func (c *PluginConfig) AddSwitch(name string) *SwitchHandle {
	if strings.TrimSpace(name) == "" {
		panic("fakes: a switch needs a name")
	}

	if c.names == nil {
		c.names = make(map[string]bool)
	}

	if c.names[name] {
		panic(fmt.Sprintf("fakes: there is already a switch called %q", name))
	}

	c.names[name] = true

	handle := &SwitchHandle{name: name}
	c.switches = append(c.switches, handle)

	return handle
}
