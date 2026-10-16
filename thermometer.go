package fakes

import (
	"math"
	"reflect"
	"sync/atomic"

	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	spi "github.com/avanha/pmaas-spi"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/environment"
	"github.com/avanha/pmaas-spi/events"
)

// thermometerType is the entity type a thermometer is registered under: the one real ones are.
var thermometerType = reflect.TypeFor[environment.IWirelessThermometer]()

const (
	// newThermometerFahrenheit is what a thermometer reads when it is first made.
	newThermometerFahrenheit = 72

	// The temperatures a fake thermometer can be set to: from colder than anywhere people live to hotter than a fire
	// alarm, which is as much as is useful, and keeps a number typed wrong from being taken for a reading.
	minFahrenheit = -100
	maxFahrenheit = 250
)

func fahrenheitToCelsius(fahrenheit float64) float32 {
	return float32((fahrenheit - 32) * 5 / 9)
}

func celsiusToFahrenheit(celsius float32) float64 {
	return float64(celsius)*9/5 + 32
}

// fakeThermometer is a thermometer with nothing behind it. It is the entity PMAAS sees, with a reading of its own,
// which it reports regularly, as a real one does, unless it has been told to stop. Owned by the plugin goroutine.
type fakeThermometer struct {
	plugin *plugin
	name   string
	id     string

	data          environment.WirelessThermometer
	stopReporting bool

	stub *thermometerStub
}

var _ environment.IWirelessThermometer = (*fakeThermometer)(nil)

func (t *fakeThermometer) GetWirelessThermometerData() environment.WirelessThermometer {
	return t.data
}

// setTemperature is the thermometer reporting a reading. A reading is news only if it differs from the last.
func (t *fakeThermometer) setTemperature(celsius float32) {
	old := t.data.SensorData

	t.data.SensorData = environment.SensorData{HasData: true, Temperature: celsius, LastUpdateTime: t.plugin.now()}

	if old.HasData && old.Temperature == celsius {
		return
	}

	t.plugin.broadcast(t.id, events.EntityStateChangedEvent{EntityEvent: t.entityEvent(), NewState: t.data})

	if old.HasData {
		t.plugin.broadcast(t.id, environment.TemperatureChangeEvent{
			EntityEvent: t.entityEvent(),
			NewValue:    celsius,
			OldValue:    old.Temperature,
		})
	}
}

// refresh is the thermometer reporting again what it read, as one does every so often: nothing has changed, so it isn't
// news, but the reading is new.
func (t *fakeThermometer) refresh() {
	if !t.stopReporting && t.data.SensorData.HasData {
		t.data.SensorData.LastUpdateTime = t.plugin.now()
	}
}

// do does something to the thermometer from the page. value is in degrees Fahrenheit, where an action takes one.
func (t *fakeThermometer) do(action common.Action, value float64) error {
	switch action {
	case common.ActionSetTemperature, common.ActionAdjustTemperature:
		if t.stopReporting {
			return common.Refuse("it has stopped reporting, so its reading can't change; resume it first")
		}

		target := value

		if action == common.ActionAdjustTemperature {
			if !t.data.SensorData.HasData {
				return common.Refuse("it has no reading to adjust")
			}

			target = celsiusToFahrenheit(t.data.SensorData.Temperature) + value
		}

		if math.IsNaN(target) || target < minFahrenheit || target > maxFahrenheit {
			return common.Refuse("the temperature must be from %d to %d degrees Fahrenheit", minFahrenheit, maxFahrenheit)
		}

		t.setTemperature(fahrenheitToCelsius(target))
	case common.ActionStopReporting:
		t.stopReporting = true
	case common.ActionResumeReporting:
		t.stopReporting = false

		// It reports at once, on coming back.
		t.refresh()
	default:
		return common.Refuse("a thermometer has no action %q", action)
	}

	t.plugin.persist()

	return nil
}

func (t *fakeThermometer) entityEvent() events.EntityEvent {
	return events.EntityEvent{Id: t.id, EntityType: thermometerType, Name: t.name}
}

func (t *fakeThermometer) getStub(container spi.IPMAASContainer) *thermometerStub {
	if t.stub == nil {
		t.stub = &thermometerStub{}
		t.stub.wrapper.Store(&spicommon.ThreadSafeEntityWrapper[environment.IWirelessThermometer]{Container: container, Entity: t})
	}

	return t.stub
}

func (t *fakeThermometer) closeStub() {
	if t.stub != nil {
		t.stub.wrapper.Store(nil)
		t.stub = nil
	}
}

// thermometerStub is the thermometer as every other goroutine sees it: each call hops onto the plugin's.
type thermometerStub struct {
	wrapper atomic.Pointer[spicommon.ThreadSafeEntityWrapper[environment.IWirelessThermometer]]
}

var _ environment.IWirelessThermometer = (*thermometerStub)(nil)

func (s *thermometerStub) GetWirelessThermometerData() environment.WirelessThermometer {
	return spicommon.ThreadSafeEntityWrapperExecValueFunc(
		s.wrapper.Load(),
		func(target environment.IWirelessThermometer) environment.WirelessThermometer {
			return target.GetWirelessThermometerData()
		})
}
