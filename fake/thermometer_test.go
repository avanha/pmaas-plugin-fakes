package fake

import (
	"sync"
	"testing"
	"time"
)

func TestThermometer(t *testing.T) {
	before := time.Now()
	th := NewThermometer("Room", 22.5)

	data := th.GetWirelessThermometerData()
	if data.Name != "Room" || !data.SensorData.HasData || data.SensorData.Temperature != 22.5 ||
		data.SensorData.LastUpdateTime.Before(before) {
		t.Errorf("data %+v", data)
	}

	th.Set(30)

	if got := th.GetWirelessThermometerData().SensorData.Temperature; got != 30 {
		t.Errorf("after Set: %v", got)
	}

	old := before.Add(-time.Hour)
	th.SetReading(10, old)

	if got := th.GetWirelessThermometerData().SensorData; got.Temperature != 10 || !got.LastUpdateTime.Equal(old) {
		t.Errorf("after SetReading: %+v", got)
	}
}

func TestSilentThermometer(t *testing.T) {
	th := NewSilentThermometer("Room")

	data := th.GetWirelessThermometerData()
	if data.Name != "Room" || data.SensorData.HasData {
		t.Errorf("data %+v", data)
	}

	th.Set(20)

	if !th.GetWirelessThermometerData().SensorData.HasData {
		t.Error("still silent after reading")
	}
}

func TestThermometer_SafeToUseFromManyGoroutines(t *testing.T) {
	th := NewThermometer("Room", 20)

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(2)

		go func() { defer wg.Done(); th.Set(21) }()
		go func() { defer wg.Done(); _ = th.GetWirelessThermometerData() }()
	}

	wg.Wait()
}
