package fake

import (
	"sync"
	"time"

	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

// Door is a device.IDoorSensor with nothing behind it, for testing code that reads a door. It is safe to use from several
// goroutines.
type Door struct {
	mu   sync.Mutex
	data device.DoorSensor
	now  func() time.Time
}

var _ device.IDoorSensor = (*Door)(nil)

// NewDoor makes a door sensor that is online and has just reported the door in the given position. DoorPositionUnknown is
// a sensor that has not reported anything yet.
func NewDoor(name string, position device.DoorPosition) *Door {
	d := &Door{now: time.Now}
	now := d.now()

	d.data = device.DoorSensor{
		Name:         name,
		Connectivity: environment.ConnectivityOnline,
		OnlineSince:  now,
	}

	if position != device.DoorPositionUnknown {
		d.data.HasData, d.data.Position, d.data.LastUpdateTime = true, position, now
	}

	return d
}

func (d *Door) GetDoorSensorData() device.DoorSensor {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.data
}

// Report is the sensor saying where the door is. The first report establishes the position, and a later one that differs
// is a change, as with a real sensor.
func (d *Door) Report(position device.DoorPosition) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	d.data.LastUpdateTime = now

	switch {
	case !d.data.HasData:
		d.data.HasData, d.data.Position = true, position
	case d.data.Position != position:
		d.data.Position, d.data.LastChangeTime = position, now
	}
}

// SetOffline takes the sensor off the network, or puts it back. An offline sensor keeps the position it last reported.
func (d *Door) SetOffline(offline bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()

	if offline {
		d.data.Connectivity, d.data.OfflineSince = environment.ConnectivityOffline, now

		return
	}

	d.data.Connectivity, d.data.OnlineSince = environment.ConnectivityOnline, now
}
