package fake

import (
	"sync"
	"time"

	"github.com/avanha/pmaas-spi/environment"
)

// Thermometer is an environment.IWirelessThermometer with nothing behind it, for testing code that reads a temperature.
// It is safe to use from several goroutines.
type Thermometer struct {
	mu   sync.Mutex
	data environment.WirelessThermometer
	now  func() time.Time
}

var _ environment.IWirelessThermometer = (*Thermometer)(nil)

// NewThermometer makes a thermometer that has just read the given temperature, in degrees Celsius, which is the unit
// entities report in.
func NewThermometer(name string, celsius float32) *Thermometer {
	t := &Thermometer{now: time.Now}
	t.data.Name = name
	t.Set(celsius)

	return t
}

// NewSilentThermometer makes a thermometer that has not reported anything.
func NewSilentThermometer(name string) *Thermometer {
	t := &Thermometer{now: time.Now}
	t.data.Name = name

	return t
}

func (t *Thermometer) GetWirelessThermometerData() environment.WirelessThermometer {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.data
}

// Set is the thermometer reading the temperature now.
func (t *Thermometer) Set(celsius float32) {
	t.SetReading(celsius, t.now())
}

// SetReading is the thermometer having read the temperature at the time given, which makes a reading as old as you
// like.
func (t *Thermometer) SetReading(celsius float32, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.data.SensorData = environment.SensorData{HasData: true, Temperature: celsius, LastUpdateTime: at}
}
