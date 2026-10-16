package fakes

import (
	"strings"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/config"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

// restoredSwitch is a fake switch the way it was left, or, if it never was (saved is the zero value), a new one that
// is off, online, and does what it is told.
func restoredSwitch(p *plugin, name string, saved config.SwitchStateV1, now time.Time) *fakeSwitch {
	s := &fakeSwitch{plugin: p, name: name}

	if saved.State == "" && !saved.HasData {
		s.data = device.Switch{
			Name:           name,
			HasData:        true,
			State:          device.SwitchStateOff,
			LastUpdateTime: now,
			Connectivity:   environment.ConnectivityOnline,
			OnlineSince:    now,
		}

		return s
	}

	s.data = device.Switch{
		Name:           name,
		HasData:        saved.HasData,
		State:          switchStateOf(saved.State),
		LastChangeTime: saved.LastChangeTime,
		LastUpdateTime: saved.LastUpdateTime,
		Connectivity:   environment.ConnectivityOnline,
		OnlineSince:    saved.OnlineSince,
		OfflineSince:   saved.OfflineSince,
	}

	if saved.Offline {
		s.data.Connectivity = environment.ConnectivityOffline
	}

	s.ignoreCommands = saved.IgnoreCommands
	s.failNext = saved.FailNext
	s.commanded = switchStateOf(saved.Commanded)
	s.commandedAt = saved.CommandedAt

	for _, c := range saved.History {
		s.history = append(s.history, command{at: c.At, state: switchStateOf(c.State), accepted: c.Accepted, result: c.Result})
	}

	return s
}

// persisted is the switch as it is kept.
func (s *fakeSwitch) persisted() config.SwitchStateV1 {
	saved := config.SwitchStateV1{
		State:          switchStateWord(s.data.State),
		HasData:        s.data.HasData,
		LastChangeTime: s.data.LastChangeTime,
		LastUpdateTime: s.data.LastUpdateTime,
		Offline:        s.data.Connectivity == environment.ConnectivityOffline,
		OfflineSince:   s.data.OfflineSince,
		OnlineSince:    s.data.OnlineSince,
		IgnoreCommands: s.ignoreCommands,
		FailNext:       s.failNext,
		Commanded:      switchStateWord(s.commanded),
		CommandedAt:    s.commandedAt,
	}

	for _, c := range s.history {
		saved.History = append(saved.History,
			config.CommandV1{At: c.at, State: switchStateWord(c.state), Accepted: c.accepted, Result: c.result})
	}

	return saved
}

// switchStateWord is a state in the words it is kept in: "on", "off" or "unknown".
func switchStateWord(state device.SwitchState) string {
	return strings.ToLower(state.String())
}

// switchStateOf is the state a word stands for. Anything it doesn't know is unknown.
func switchStateOf(word string) device.SwitchState {
	switch word {
	case "on":
		return device.SwitchStateOn
	case "off":
		return device.SwitchStateOff
	default:
		return device.SwitchStateUnknown
	}
}

// restoredThermometer is a fake thermometer the way it was left, or, if it never was (found is false), a new one
// that reads 72°F. A thermometer that was reporting reports again as it starts, as a real one does on reconnecting; one
// that had stopped is as stale as it was.
func restoredThermometer(p *plugin, name string, saved config.ThermometerStateV1, found bool, now time.Time) *fakeThermometer {
	t := &fakeThermometer{plugin: p, name: name}
	t.data.Name = name

	if !found {
		t.data.SensorData = environment.SensorData{
			HasData:        true,
			Temperature:    fahrenheitToCelsius(newThermometerFahrenheit),
			LastUpdateTime: now,
		}

		return t
	}

	t.stopReporting = saved.StopReporting
	t.data.SensorData = environment.SensorData{
		HasData:        saved.HasData,
		Temperature:    saved.Celsius,
		LastUpdateTime: saved.LastUpdateTime,
	}

	t.refresh()

	return t
}

// persisted is the thermometer as it is kept.
func (t *fakeThermometer) persisted() config.ThermometerStateV1 {
	return config.ThermometerStateV1{
		HasData:        t.data.SensorData.HasData,
		Celsius:        t.data.SensorData.Temperature,
		StopReporting:  t.stopReporting,
		LastUpdateTime: t.data.SensorData.LastUpdateTime,
	}
}
