package fake

import (
	"sync"
	"testing"

	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

func TestNewDoor(t *testing.T) {
	closed := NewDoor("Garage", device.DoorPositionClosed).GetDoorSensorData()
	if closed.Name != "Garage" || !closed.HasData || closed.Position != device.DoorPositionClosed ||
		closed.Connectivity != environment.ConnectivityOnline || closed.LastUpdateTime.IsZero() || !closed.LastChangeTime.IsZero() {
		t.Errorf("closed: %+v", closed)
	}

	unreported := NewDoor("Garage", device.DoorPositionUnknown).GetDoorSensorData()
	if unreported.HasData || unreported.Position != device.DoorPositionUnknown || !unreported.LastUpdateTime.IsZero() {
		t.Errorf("unreported: %+v", unreported)
	}
}

func TestDoor_ReportsChangesAndTheFirstReportIsNotOne(t *testing.T) {
	d := NewDoor("Garage", device.DoorPositionUnknown)
	d.Report(device.DoorPositionOpen)

	if data := d.GetDoorSensorData(); !data.HasData || data.Position != device.DoorPositionOpen || !data.LastChangeTime.IsZero() {
		t.Fatalf("first report: %+v", data)
	}

	d.Report(device.DoorPositionClosed)

	data := d.GetDoorSensorData()
	if data.Position != device.DoorPositionClosed || data.LastChangeTime.IsZero() {
		t.Fatalf("a change: %+v", data)
	}

	changed := data.LastChangeTime

	d.Report(device.DoorPositionClosed)

	if again := d.GetDoorSensorData(); !again.LastChangeTime.Equal(changed) {
		t.Errorf("the same position moved the change time: %v, was %v", again.LastChangeTime, changed)
	}
}

func TestDoor_SetOffline(t *testing.T) {
	d := NewDoor("Garage", device.DoorPositionOpen)
	d.SetOffline(true)

	data := d.GetDoorSensorData()
	if data.Connectivity != environment.ConnectivityOffline || data.OfflineSince.IsZero() || data.Position != device.DoorPositionOpen {
		t.Errorf("offline: %+v", data)
	}

	d.SetOffline(false)

	if data := d.GetDoorSensorData(); data.Connectivity != environment.ConnectivityOnline {
		t.Errorf("online: %+v", data)
	}
}

func TestDoor_SafeToUseFromManyGoroutines(t *testing.T) {
	d := NewDoor("Garage", device.DoorPositionClosed)

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(3)

		go func() { defer wg.Done(); d.Report(device.DoorPositionOpen) }()
		go func() { defer wg.Done(); _ = d.GetDoorSensorData() }()
		go func() { defer wg.Done(); d.SetOffline(true); d.SetOffline(false) }()
	}

	wg.Wait()
}
