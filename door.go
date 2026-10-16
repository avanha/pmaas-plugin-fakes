package fakes

import (
	"time"

	"sync/atomic"

	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi/alert"
	spi "github.com/avanha/pmaas-spi"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
	"github.com/avanha/pmaas-spi/events"
)

// fakeDoorSensor is a door sensor with nothing behind it. It is the entity PMAAS sees, with a position of its own, which
// the page changes as a door moving would, and it can go off the network and keep the position it last reported, as a
// real one does. Owned by the plugin goroutine.
type fakeDoorSensor struct {
	plugin *plugin
	name   string
	id     string

	data device.DoorSensor

	// offlineWatch holds the alert about the sensor being offline, if one was asked for.
	offlineWatch *alert.OfflineWatch

	stub *doorStub
}

var _ device.IDoorSensor = (*fakeDoorSensor)(nil)

func (d *fakeDoorSensor) GetDoorSensorData() device.DoorSensor {
	return d.data
}

// report is the sensor saying where the door is. It does what a real sensor's report does: the first establishes the
// position without being a change, and one that is the same as before is not news.
func (d *fakeDoorSensor) report(position device.DoorPosition) {
	now := d.plugin.now()
	old := d.data

	d.data.LastUpdateTime = now

	switch {
	case !old.HasData:
		d.data.HasData = true
		d.data.Position = position
	case old.Position != position:
		d.data.Position = position
		d.data.LastChangeTime = now
	default:
		return
	}

	d.plugin.broadcast(d.id, events.EntityStateChangedEvent{EntityEvent: d.entityEvent(), NewState: d.data})

	if old.HasData {
		d.plugin.broadcast(d.id, device.DoorPositionChangedEvent{
			EntityEvent: d.entityEvent(),
			NewPosition: position,
			OldPosition: old.Position,
		})
	}
}

// setOffline takes the sensor off the network, or puts it back. A sensor that is off keeps the position it last reported,
// and says it is offline.
func (d *fakeDoorSensor) setOffline(offline bool) {
	if offline == (d.data.Connectivity == environment.ConnectivityOffline) {
		return
	}

	now := d.plugin.now()

	if offline {
		d.data.Connectivity, d.data.OfflineSince = environment.ConnectivityOffline, now
	} else {
		d.data.Connectivity, d.data.OnlineSince = environment.ConnectivityOnline, now
	}

	d.plugin.broadcast(d.id, events.EntityStateChangedEvent{EntityEvent: d.entityEvent(), NewState: d.data})
	d.checkOffline()
}

// checkOffline raises, escalates or clears the alert about the sensor being offline, if there is to be one.
func (d *fakeDoorSensor) checkOffline() {
	d.plugin.checkOffline(d.offlineWatch, d.data.Connectivity, d.data.OfflineSince)
}

// do does something to the sensor from the page.
func (d *fakeDoorSensor) do(action common.Action) error {
	switch action {
	case common.ActionDoorOpen, common.ActionDoorClose:
		if d.data.Connectivity == environment.ConnectivityOffline {
			return common.Refuse("the sensor is offline, so it can't report the door moving")
		}

		if action == common.ActionDoorOpen {
			d.report(device.DoorPositionOpen)
		} else {
			d.report(device.DoorPositionClosed)
		}
	case common.ActionOffline:
		d.setOffline(true)
	case common.ActionOnline:
		d.setOffline(false)
	default:
		return common.Refuse("a door sensor has no action %q", action)
	}

	d.plugin.persist()

	return nil
}

func (d *fakeDoorSensor) entityEvent() events.EntityEvent {
	return events.EntityEvent{Id: d.id, EntityType: device.IDoorSensorType, Name: d.name}
}

func (d *fakeDoorSensor) getStub(container spi.IPMAASContainer) *doorStub {
	if d.stub == nil {
		d.stub = &doorStub{}
		d.stub.wrapper.Store(&spicommon.ThreadSafeEntityWrapper[device.IDoorSensor]{Container: container, Entity: d})
	}

	return d.stub
}

func (d *fakeDoorSensor) closeStub() {
	if d.stub != nil {
		d.stub.wrapper.Store(nil)
		d.stub = nil
	}
}

// doorStub is the sensor as every other goroutine sees it: each call hops onto the plugin's.
type doorStub struct {
	wrapper atomic.Pointer[spicommon.ThreadSafeEntityWrapper[device.IDoorSensor]]
}

var _ device.IDoorSensor = (*doorStub)(nil)

func (s *doorStub) GetDoorSensorData() device.DoorSensor {
	return spicommon.ThreadSafeEntityWrapperExecValueFunc(
		s.wrapper.Load(),
		func(target device.IDoorSensor) device.DoorSensor { return target.GetDoorSensorData() })
}

// fakeHost is a host that is up until the page takes it down. The assembly's handle answers netmon from any goroutine, so
// the state it answers with is mirrored there. Owned by the plugin goroutine.
type fakeHost struct {
	plugin *plugin
	name   string
	handle *HostHandle

	down  bool
	since time.Time
}

func (h *fakeHost) do(action common.Action) error {
	switch action {
	case common.ActionHostDown:
		h.set(true)
	case common.ActionHostUp:
		h.set(false)
	default:
		return common.Refuse("a host has no action %q", action)
	}

	h.plugin.persist()

	return nil
}

// set takes the host down, or brings it back. Nothing changes if it already is.
func (h *fakeHost) set(down bool) {
	if h.down == down {
		return
	}

	h.down, h.since = down, h.plugin.now()
	h.handle.down.Store(down)
}
