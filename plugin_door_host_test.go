package fakes

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/config"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
	"github.com/avanha/pmaas-spi/events"
)

func (f *fixture) doorData() device.DoorSensor {
	f.t.Helper()

	return f.door.GetDoorSensorData()
}

func doorChanges(broadcasts []any) []device.DoorPositionChangedEvent {
	var result []device.DoorPositionChangedEvent

	for _, b := range broadcasts {
		if c, ok := b.(device.DoorPositionChangedEvent); ok {
			result = append(result, c)
		}
	}

	return result
}

func doorStates(broadcasts []any) []device.DoorSensor {
	var result []device.DoorSensor

	for _, b := range broadcasts {
		if c, ok := b.(events.EntityStateChangedEvent); ok {
			if data, isDoor := c.NewState.(device.DoorSensor); isDoor {
				result = append(result, data)
			}
		}
	}

	return result
}

func TestDoorSensor_RegisteredAsTheEntityARealOneIsAndStartsClosedAndOnline(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	r := f.container.registrations[2]
	if r.entityType != device.IDoorSensorType || r.name != "Garage door" || r.uniqueData != "Garage door" {
		t.Errorf("registration: %+v", r)
	}

	stub, err := r.stubFactory()
	if err != nil {
		t.Fatal(err)
	}

	data := stub.(device.IDoorSensor).GetDoorSensorData()
	if data.Name != "Garage door" || !data.HasData || data.Position != device.DoorPositionClosed ||
		data.Connectivity != environment.ConnectivityOnline || !data.OnlineSince.Equal(epoch) {
		t.Errorf("a new fake: %+v", data)
	}

	if saved := f.container.lastSaved(t).DoorSensors["Garage door"]; saved.Position != "closed" || !saved.HasData {
		t.Errorf("kept %+v", saved)
	}

	if f.door.Name() != "Garage door" {
		t.Errorf("name %q", f.door.Name())
	}
}

func TestDoorSensorHandle_IsASensorBeforeThePluginStartsAndAfterItStops(t *testing.T) {
	f := newFixture(t, nil)

	if got := f.door.GetDoorSensorData(); got.Name != "Garage door" || got.HasData {
		t.Errorf("before: %+v", got)
	}

	f.start()

	if got := f.door.GetDoorSensorData(); !got.HasData {
		t.Errorf("while running: %+v", got)
	}

	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	if got := f.door.GetDoorSensorData(); got.HasData || got.Name != "Garage door" {
		t.Errorf("after: %+v", got)
	}
}

func TestDoorSensor_OpeningAndClosingAreChangesThatAreNewsAndAreKept(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	f.advance(time.Minute)

	if err := f.doTo("Garage door", common.ActionDoorOpen, 0); err != nil {
		t.Fatal(err)
	}

	data := f.doorData()
	if data.Position != device.DoorPositionOpen || !data.LastChangeTime.Equal(f.clock) || !data.LastUpdateTime.Equal(f.clock) {
		t.Fatalf("open: %+v", data)
	}

	broadcasts := f.container.broadcastsOf()

	changes := doorChanges(broadcasts)
	if len(changes) != 1 || changes[0].NewPosition != device.DoorPositionOpen || changes[0].OldPosition != device.DoorPositionClosed ||
		changes[0].Name != "Garage door" || changes[0].EntityType != device.IDoorSensorType {
		t.Errorf("changes %+v", changes)
	}

	if states := doorStates(broadcasts); len(states) != 1 || states[0].Position != device.DoorPositionOpen {
		t.Errorf("states %+v", states)
	}

	if saved := f.container.lastSaved(t).DoorSensors["Garage door"]; saved.Position != "open" || !saved.LastChangeTime.Equal(f.clock) {
		t.Errorf("kept %+v", saved)
	}

	f.advance(time.Minute)
	_ = f.doTo("Garage door", common.ActionDoorClose, 0)

	if data := f.doorData(); data.Position != device.DoorPositionClosed || !data.LastChangeTime.Equal(f.clock) {
		t.Errorf("closed: %+v", data)
	}
}

func TestDoorSensor_TheSamePositionAgainIsNotNewsButIsNewer(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	_ = f.doTo("Garage door", common.ActionDoorOpen, 0)

	before := len(f.container.broadcastsOf())
	changedAt := f.doorData().LastChangeTime

	f.advance(time.Minute)
	_ = f.doTo("Garage door", common.ActionDoorOpen, 0)

	if len(f.container.broadcastsOf()) != before {
		t.Error("an unchanged position was broadcast")
	}

	if data := f.doorData(); !data.LastUpdateTime.Equal(f.clock) || !data.LastChangeTime.Equal(changedAt) {
		t.Errorf("data %+v, change time was %v", data, changedAt)
	}
}

func TestDoorSensor_TheFirstReportOfOneThatHadNoneIsNotAChange(t *testing.T) {
	f := newFixture(t, &config.PersistentConfigV1{DoorSensors: map[string]config.DoorSensorStateV1{"Garage door": {}}})
	f.start()

	if f.doorData().HasData {
		t.Fatal("restored with a position it didn't have")
	}

	_ = f.doTo("Garage door", common.ActionDoorOpen, 0)

	broadcasts := f.container.broadcastsOf()

	if changes := doorChanges(broadcasts); len(changes) != 0 {
		t.Errorf("the first report was a change: %+v", changes)
	}

	if data := f.doorData(); !data.HasData || data.Position != device.DoorPositionOpen || !data.LastChangeTime.IsZero() {
		t.Errorf("data %+v", data)
	}
}

func TestDoorSensor_OfflineKeepsThePositionAndCanNotReportTheDoorMoving(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	_ = f.doTo("Garage door", common.ActionDoorOpen, 0)

	f.advance(time.Minute)

	if err := f.doTo("Garage door", common.ActionOffline, 0); err != nil {
		t.Fatal(err)
	}

	data := f.doorData()
	if data.Connectivity != environment.ConnectivityOffline || !data.OfflineSince.Equal(f.clock) || data.Position != device.DoorPositionOpen {
		t.Errorf("offline: %+v", data)
	}

	var refused *common.RequestError
	for _, action := range []common.Action{common.ActionDoorOpen, common.ActionDoorClose} {
		if err := f.doTo("Garage door", action, 0); !errors.As(err, &refused) {
			t.Errorf("%s while offline: %v", action, err)
		}
	}

	if saved := f.container.lastSaved(t).DoorSensors["Garage door"]; !saved.Offline || saved.Position != "open" {
		t.Errorf("kept %+v", saved)
	}

	// Taking it offline when it is: nothing new.
	before := len(f.container.broadcastsOf())
	_ = f.doTo("Garage door", common.ActionOffline, 0)

	if len(f.container.broadcastsOf()) != before {
		t.Error("taking an offline sensor offline was broadcast")
	}

	f.advance(time.Minute)

	if err := f.doTo("Garage door", common.ActionOnline, 0); err != nil {
		t.Fatal(err)
	}

	if data := f.doorData(); data.Connectivity != environment.ConnectivityOnline || !data.OnlineSince.Equal(f.clock) {
		t.Errorf("online: %+v", data)
	}

	if err := f.doTo("Garage door", common.ActionDoorClose, 0); err != nil {
		t.Errorf("back online: %v", err)
	}
}

func TestDoorSensor_RefusesWhatCanNotBeDone(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	var refused *common.RequestError

	for _, action := range []common.Action{"dance", common.ActionSetTemperature, common.ActionHostDown, common.ActionStopReporting} {
		if err := f.doTo("Garage door", action, 0); !errors.As(err, &refused) {
			t.Errorf("%s: %v", action, err)
		}
	}
}

func TestDoorSensor_RestartComesBackAsLeft(t *testing.T) {
	first := newFixture(t, nil)
	first.start()
	first.advance(time.Minute)
	_ = first.doTo("Garage door", common.ActionDoorOpen, 0)
	_ = first.doTo("Garage door", common.ActionOffline, 0)

	left := first.container.lastSaved(t)

	second := newFixture(t, &left)
	second.clock = epoch.Add(time.Hour)
	second.start()

	data := second.doorData()
	if data.Position != device.DoorPositionOpen || data.Connectivity != environment.ConnectivityOffline ||
		!data.LastChangeTime.Equal(epoch.Add(time.Minute)) {
		t.Errorf("data %+v", data)
	}

	if got, want := second.container.lastSaved(t).DoorSensors["Garage door"], left.DoorSensors["Garage door"]; got != want {
		t.Errorf("after a restart:\n got  %+v\n want %+v", got, want)
	}
}

func TestDoorSensor_AWordItDoesNotKnowIsAnUnknownPosition(t *testing.T) {
	f := newFixture(t, &config.PersistentConfigV1{DoorSensors: map[string]config.DoorSensorStateV1{
		"Garage door": {Position: "ajar", HasData: true},
	}})
	f.start()

	if data := f.doorData(); data.Position != device.DoorPositionUnknown || data.Connectivity != environment.ConnectivityOnline {
		t.Errorf("data %+v", data)
	}

	for word, want := range map[string]device.DoorPosition{
		"open": device.DoorPositionOpen, "closed": device.DoorPositionClosed, "unknown": device.DoorPositionUnknown, "": device.DoorPositionUnknown,
	} {
		if got := doorPositionOf(word); got != want {
			t.Errorf("%q: %v", word, got)
		}

		if want != device.DoorPositionUnknown || word == "unknown" {
			if got := doorPositionWord(want); got != word {
				t.Errorf("%v: %q, want %q", want, got, word)
			}
		}
	}
}

func TestDoorSensor_AStubTakenBeforeStopFailsFast(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	stub, err := f.container.registrations[2].stubFactory()
	if err != nil {
		t.Fatal(err)
	}

	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	defer func() {
		if recover() == nil {
			t.Error("a stub of a door sensor that has been deregistered still worked")
		}
	}()

	stub.(device.IDoorSensor).GetDoorSensorData()
}

func TestHost_IsReachableUntilTheyAreTakenDownAndComesBack(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	if !f.host.Reachable(context.Background()) || f.host.Name() != "Router" {
		t.Fatal("not reachable to begin with")
	}

	f.advance(time.Minute)

	if err := f.doTo("Router", common.ActionHostDown, 0); err != nil {
		t.Fatal(err)
	}

	if f.host.Reachable(context.Background()) {
		t.Error("reachable when down")
	}

	saved := f.container.lastSaved(t).Hosts["Router"]
	if !saved.Down || !saved.Since.Equal(f.clock) {
		t.Errorf("kept %+v", saved)
	}

	// Taking it down again doesn't move when it went down.
	f.advance(time.Minute)
	_ = f.doTo("Router", common.ActionHostDown, 0)

	if again := f.container.lastSaved(t).Hosts["Router"]; !again.Since.Equal(saved.Since) {
		t.Errorf("down since moved: %v, was %v", again.Since, saved.Since)
	}

	if err := f.doTo("Router", common.ActionHostUp, 0); err != nil {
		t.Fatal(err)
	}

	if !f.host.Reachable(context.Background()) {
		t.Error("not reachable when up")
	}

	if up := f.container.lastSaved(t).Hosts["Router"]; up.Down || !up.Since.Equal(f.clock) {
		t.Errorf("kept %+v", up)
	}
}

func TestHost_RestartComesBackAsLeftBeforeAnyoneAsks(t *testing.T) {
	first := newFixture(t, nil)
	first.start()
	_ = first.doTo("Router", common.ActionHostDown, 0)

	left := first.container.lastSaved(t)

	second := newFixture(t, &left)
	second.start()

	if second.host.Reachable(context.Background()) {
		t.Error("a host that was down is up after a restart")
	}

	if shown, _ := (&fakeStoreAdapter{parent: second.plugin}).GetFakes(); len(shown.Hosts) != 1 || !shown.Hosts[0].Down {
		t.Errorf("shown %+v", shown.Hosts)
	}
}

func TestHost_RefusesWhatCanNotBeDone(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	var refused *common.RequestError

	for _, action := range []common.Action{"dance", common.ActionOffline, common.ActionDoorOpen} {
		if err := f.doTo("Router", action, 0); !errors.As(err, &refused) {
			t.Errorf("%s: %v", action, err)
		}
	}
}

func TestHost_CanBeAskedFromAnyGoroutineWhileItChanges(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(2)

		go func() { defer wg.Done(); _ = f.host.Reachable(context.Background()) }()
		go func() {
			defer wg.Done()
			_ = f.doTo("Router", common.ActionHostDown, 0)
			_ = f.doTo("Router", common.ActionHostUp, 0)
		}()
	}

	wg.Wait()
}

func TestGetFakes_ShowsTheDoorAndTheHostToo(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	_ = f.doTo("Garage door", common.ActionDoorOpen, 0)
	_ = f.doTo("Router", common.ActionHostDown, 0)

	shown, err := (&fakeStoreAdapter{parent: f.plugin}).GetFakes()
	if err != nil {
		t.Fatal(err)
	}

	if len(shown.DoorSensors) != 1 || len(shown.Hosts) != 1 {
		t.Fatalf("shown %+v", shown)
	}

	if door := shown.DoorSensors[0]; door.Name != "Garage door" || door.Position != "Open" || door.PositionClass != "open" || door.Offline ||
		!door.LastChangeTime.Equal(epoch) {
		t.Errorf("door %+v", door)
	}

	if host := shown.Hosts[0]; host.Name != "Router" || !host.Down || !host.Since.Equal(epoch) {
		t.Errorf("host %+v", host)
	}
}

func TestConfig_DoorSensorsAndHostsShareTheNamesOfTheOtherFakes(t *testing.T) {
	conf := NewPluginConfig()
	conf.AddSwitch("Porch")
	conf.AddDoorSensor("Garage door")
	conf.AddHost("Router")

	for name, add := range map[string]func(){
		"a door named like a switch": func() { conf.AddDoorSensor("Porch") },
		"a host named like a door":   func() { conf.AddHost("Garage door") },
		"a switch named like a host": func() { conf.AddSwitch("Router") },
		"an empty door name":         func() { conf.AddDoorSensor("") },
		"an empty host name":         func() { conf.AddHost(" ") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: didn't panic", name)
				}
			}()

			add()
		}()
	}
}

func TestDoorSensor_ADoorLeftOpenOnTheVirtualClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, nil)
		f.plugin.now = time.Now
		f.start()

		time.Sleep(time.Hour)

		_ = f.doTo("Garage door", common.ActionDoorOpen, 0)
		openedAt := time.Now()

		time.Sleep(7 * time.Minute)

		data := f.doorData()
		if data.Position != device.DoorPositionOpen || !data.LastChangeTime.Equal(openedAt) || time.Since(data.LastChangeTime) != 7*time.Minute {
			t.Errorf("data %+v, open for %v", data, time.Since(data.LastChangeTime))
		}

		f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })
	})
}

func TestHost_ANewOneIsUpSinceItStarted(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	shown, _ := (&fakeStoreAdapter{parent: f.plugin}).GetFakes()
	if len(shown.Hosts) != 1 || shown.Hosts[0].Down || !shown.Hosts[0].Since.Equal(epoch) {
		t.Errorf("shown %+v", shown.Hosts)
	}
}
