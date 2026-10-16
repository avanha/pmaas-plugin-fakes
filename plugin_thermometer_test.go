package fakes

import (
	"errors"
	"math"
	"testing"
	"testing/synctest"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/config"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi/environment"
	"github.com/avanha/pmaas-spi/events"
)

func near(got float32, want float64) bool {
	return math.Abs(float64(got)-want) < 0.01
}

// doTo does something to the named fake from the page, where the plugin's code does, and returns what it said.
func (f *fixture) doTo(name string, action common.Action, value float64) error {
	f.t.Helper()

	var err error

	f.container.onPluginGoroutine(func() { err = f.plugin.do(name, action, 0, value) })

	return err
}

func (f *fixture) reading() environment.SensorData {
	f.t.Helper()

	return f.thermo.GetWirelessThermometerData().SensorData
}

func temperatureChanges(broadcasts []any) []environment.TemperatureChangeEvent {
	var result []environment.TemperatureChangeEvent

	for _, b := range broadcasts {
		if c, ok := b.(environment.TemperatureChangeEvent); ok {
			result = append(result, c)
		}
	}

	return result
}

func thermometerStates(broadcasts []any) []environment.WirelessThermometer {
	var result []environment.WirelessThermometer

	for _, b := range broadcasts {
		if c, ok := b.(events.EntityStateChangedEvent); ok {
			if data, isThermometer := c.NewState.(environment.WirelessThermometer); isThermometer {
				result = append(result, data)
			}
		}
	}

	return result
}

func TestThermometer_RegisteredAsTheEntityARealOneIsAndStartsAt72(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	r := f.container.registrations[1]
	if r.entityType != thermometerType || r.name != "Room" || r.uniqueData != "Room" {
		t.Errorf("registration: %+v", r)
	}

	stub, err := r.stubFactory()
	if err != nil {
		t.Fatal(err)
	}

	data := stub.(environment.IWirelessThermometer).GetWirelessThermometerData()
	if data.Name != "Room" || !data.SensorData.HasData || !near(data.SensorData.Temperature, 22.2222) ||
		!data.SensorData.LastUpdateTime.Equal(epoch) {
		t.Errorf("a new fake: %+v", data)
	}

	saved := f.container.lastSaved(t).Thermometers["Room"]
	if !saved.HasData || !near(saved.Celsius, 22.2222) || saved.StopReporting {
		t.Errorf("kept %+v", saved)
	}
}

func TestThermometerHandle_IsAThermometerBeforeThePluginStartsAndAfterItStops(t *testing.T) {
	f := newFixture(t, nil)

	if got := f.thermo.GetWirelessThermometerData(); got.Name != "Room" || got.SensorData.HasData {
		t.Errorf("before: %+v", got)
	}

	f.start()

	if got := f.thermo.GetWirelessThermometerData(); !got.SensorData.HasData {
		t.Errorf("while running: %+v", got)
	}

	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	if got := f.thermo.GetWirelessThermometerData(); got.SensorData.HasData || got.Name != "Room" {
		t.Errorf("after: %+v", got)
	}

	if f.thermo.Name() != "Room" {
		t.Errorf("name %q", f.thermo.Name())
	}
}

func TestThermometer_SetTemperatureIsAReadingThatIsNewsAndIsKept(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	f.advance(time.Minute)

	if err := f.doTo("Room", common.ActionSetTemperature, 78.1); err != nil {
		t.Fatal(err)
	}

	reading := f.reading()
	if !near(reading.Temperature, 25.6111) || !reading.LastUpdateTime.Equal(f.clock) || !reading.HasData {
		t.Errorf("reading %+v", reading)
	}

	broadcasts := f.container.broadcastsOf()

	changes := temperatureChanges(broadcasts)
	if len(changes) != 1 || !near(changes[0].NewValue, 25.6111) || !near(changes[0].OldValue, 22.2222) ||
		changes[0].Name != "Room" || changes[0].EntityType != thermometerType {
		t.Errorf("changes %+v", changes)
	}

	if states := thermometerStates(broadcasts); len(states) != 1 || !near(states[0].SensorData.Temperature, 25.6111) {
		t.Errorf("states %+v", states)
	}

	if saved := f.container.lastSaved(t).Thermometers["Room"]; !near(saved.Celsius, 25.6111) || !saved.LastUpdateTime.Equal(f.clock) {
		t.Errorf("kept %+v", saved)
	}
}

func TestThermometer_TheSameReadingIsNotNewsButIsNewer(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	_ = f.doTo("Room", common.ActionSetTemperature, 72)

	before := len(f.container.broadcastsOf())

	f.advance(time.Minute)
	_ = f.doTo("Room", common.ActionSetTemperature, 72)

	if len(f.container.broadcastsOf()) != before {
		t.Error("an unchanged reading was broadcast")
	}

	if !f.reading().LastUpdateTime.Equal(f.clock) {
		t.Errorf("reading %+v", f.reading())
	}
}

func TestThermometer_AdjustChangesTheReadingByDegreesFahrenheit(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	_ = f.doTo("Room", common.ActionSetTemperature, 75)

	if err := f.doTo("Room", common.ActionAdjustTemperature, -1.5); err != nil {
		t.Fatal(err)
	}

	if got := celsiusToFahrenheit(f.reading().Temperature); math.Abs(got-73.5) > 0.01 {
		t.Errorf("got %v°F", got)
	}

	_ = f.doTo("Room", common.ActionAdjustTemperature, 1)

	if got := celsiusToFahrenheit(f.reading().Temperature); math.Abs(got-74.5) > 0.01 {
		t.Errorf("got %v°F", got)
	}
}

func TestThermometer_RefusesWhatCanNotBeDone(t *testing.T) {
	f := newFixture(t, &config.PersistentConfigV1{Thermometers: map[string]config.ThermometerStateV1{"Room": {}}})
	f.start()

	var refused *common.RequestError

	// No reading to adjust.
	if err := f.doTo("Room", common.ActionAdjustTemperature, 1); !errors.As(err, &refused) {
		t.Errorf("adjusting nothing: %v", err)
	}

	_ = f.doTo("Room", common.ActionSetTemperature, 70)

	for name, value := range map[string]float64{
		"nan": math.NaN(), "infinite": math.Inf(1), "too cold": minFahrenheit - 1, "too hot": maxFahrenheit + 1,
	} {
		if err := f.doTo("Room", common.ActionSetTemperature, value); !errors.As(err, &refused) {
			t.Errorf("%s: %v", name, err)
		}
	}

	for _, value := range []float64{minFahrenheit, maxFahrenheit} {
		if err := f.doTo("Room", common.ActionSetTemperature, value); err != nil {
			t.Errorf("%v: %v", value, err)
		}
	}

	// Adjusting past the end.
	_ = f.doTo("Room", common.ActionSetTemperature, maxFahrenheit)

	if err := f.doTo("Room", common.ActionAdjustTemperature, 1); !errors.As(err, &refused) {
		t.Errorf("adjusting past the end: %v", err)
	}

	if err := f.doTo("Room", "dance", 0); !errors.As(err, &refused) {
		t.Errorf("an unknown action: %v", err)
	}

	// Switch actions mean nothing to a thermometer.
	if err := f.doTo("Room", common.ActionOffline, 0); !errors.As(err, &refused) {
		t.Errorf("a switch action: %v", err)
	}
}

func TestThermometer_StoppedReportingItsReadingGetsOlderAndCanNotChange(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	_ = f.doTo("Room", common.ActionSetTemperature, 70)
	reported := f.reading().LastUpdateTime

	if err := f.doTo("Room", common.ActionStopReporting, 0); err != nil {
		t.Fatal(err)
	}

	f.advance(time.Hour)
	f.container.onPluginGoroutine(f.plugin.report)

	if got := f.reading().LastUpdateTime; !got.Equal(reported) {
		t.Errorf("a thermometer that stopped reporting reported: %v, was %v", got, reported)
	}

	var refused *common.RequestError
	for _, action := range []common.Action{common.ActionSetTemperature, common.ActionAdjustTemperature} {
		if err := f.doTo("Room", action, 70); !errors.As(err, &refused) {
			t.Errorf("%s while stopped: %v", action, err)
		}
	}

	if saved := f.container.lastSaved(t).Thermometers["Room"]; !saved.StopReporting || !saved.LastUpdateTime.Equal(reported) {
		t.Errorf("kept %+v", saved)
	}

	// Back, it reports at once.
	if err := f.doTo("Room", common.ActionResumeReporting, 0); err != nil {
		t.Fatal(err)
	}

	if got := f.reading().LastUpdateTime; !got.Equal(f.clock) {
		t.Errorf("after resuming: %v, want %v", got, f.clock)
	}

	if err := f.doTo("Room", common.ActionSetTemperature, 71); err != nil {
		t.Errorf("after resuming: %v", err)
	}
}

func TestThermometer_ReportingKeepsTheReadingNewWithoutMakingNews(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	before := len(f.container.broadcastsOf())
	saves := f.container.saveCount()

	f.advance(5 * time.Minute)
	f.container.onPluginGoroutine(f.plugin.report)

	if got := f.reading().LastUpdateTime; !got.Equal(f.clock) {
		t.Errorf("reading %v, want %v", got, f.clock)
	}

	if len(f.container.broadcastsOf()) != before || f.container.saveCount() != saves {
		t.Error("a report with nothing new was broadcast or kept")
	}
}

func TestThermometer_ReportsEveryMinuteOnItsOwnWhileRunningAndStopsWhenThePluginDoes(t *testing.T) {
	// On the virtual clock of a bubble, ten minutes pass at once, and exactly.
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, nil)
		f.plugin.now = time.Now
		f.start()

		started := f.reading().LastUpdateTime

		time.Sleep(10*time.Minute + 30*time.Second)
		synctest.Wait()

		// It reported at each of the ten minutes, the last of them half a minute ago.
		if got, want := f.reading().LastUpdateTime, started.Add(10*time.Minute); !got.Equal(want) {
			t.Errorf("last reported %v, want %v", got, want)
		}

		var done chan func()

		f.container.onPluginGoroutine(func() { done = f.plugin.Stop() })

		if _, open := <-done; open {
			t.Fatal("received a callback, wanted the channel closed")
		}

		// And nothing is left running: the bubble would not end while a goroutine was still waiting for a tick.
	})
}

func TestThermometer_AThermometerThatStoppedReportingGetsOlderOnTheClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, nil)
		f.plugin.now = time.Now
		f.start()

		reported := f.reading().LastUpdateTime

		if err := f.doTo("Room", common.ActionStopReporting, 0); err != nil {
			t.Fatal(err)
		}

		time.Sleep(2 * time.Hour)
		synctest.Wait()

		if got := f.reading().LastUpdateTime; !got.Equal(reported) {
			t.Errorf("a thermometer that stopped reporting reported: %v, was %v", got, reported)
		}

		if err := f.doTo("Room", common.ActionResumeReporting, 0); err != nil {
			t.Fatal(err)
		}

		if got := f.reading().LastUpdateTime; !got.Equal(reported.Add(2 * time.Hour)) {
			t.Errorf("after resuming: %v, want %v", got, reported.Add(2*time.Hour))
		}

		f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })
	})
}

func TestThermometer_NoThermometersMeansNothingToReportWith(t *testing.T) {
	container := newFakeContainer(t)
	conf := NewPluginConfig()
	conf.AddSwitch("Porch")

	p := NewPlugin(conf).(*plugin)

	container.onPluginGoroutine(func() { p.Init(container); p.Start() })

	if p.cancelReporting != nil {
		t.Error("reporting was started with no thermometers")
	}

	container.onPluginGoroutine(func() { <-p.Stop() })
}

func TestThermometer_RestartComesBackAsLeft(t *testing.T) {
	first := newFixture(t, nil)
	first.start()
	first.advance(time.Minute)
	_ = first.doTo("Room", common.ActionSetTemperature, 66)
	_ = first.doTo("Room", common.ActionStopReporting, 0)

	left := first.container.lastSaved(t)

	second := newFixture(t, &left)
	second.clock = epoch.Add(time.Hour)
	second.start()

	reading := second.reading()
	if !near(reading.Temperature, 18.8889) || !reading.LastUpdateTime.Equal(epoch.Add(time.Minute)) {
		t.Errorf("a thermometer that had stopped is as stale as it was: %+v", reading)
	}

	// One that was reporting reports again as it starts.
	running := newFixture(t, &config.PersistentConfigV1{Thermometers: map[string]config.ThermometerStateV1{
		"Room": {HasData: true, Celsius: 20, LastUpdateTime: epoch.Add(-time.Hour)},
	}})
	running.start()

	if got := running.reading(); !near(got.Temperature, 20) || !got.LastUpdateTime.Equal(epoch) {
		t.Errorf("one that was reporting: %+v", got)
	}
}

func TestThermometer_AStubTakenBeforeStopFailsFast(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	stub, err := f.container.registrations[1].stubFactory()
	if err != nil {
		t.Fatal(err)
	}

	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	defer func() {
		if recover() == nil {
			t.Error("a stub of a thermometer that has been deregistered still worked")
		}
	}()

	stub.(environment.IWirelessThermometer).GetWirelessThermometerData()
}

func TestGetFakes_ShowsTheThermometerToo(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	_ = f.doTo("Room", common.ActionSetTemperature, 80)
	_ = f.doTo("Room", common.ActionStopReporting, 0)

	shown, err := (&fakeStoreAdapter{parent: f.plugin}).GetFakes()
	if err != nil {
		t.Fatal(err)
	}

	if len(shown.Thermometers) != 1 || len(shown.Switches) != 1 {
		t.Fatalf("shown %+v", shown)
	}

	item := shown.Thermometers[0]
	if item.Name != "Room" || !item.HasData || math.Abs(float64(item.Fahrenheit)-80) > 0.01 ||
		!near(item.Celsius, 26.6667) || !item.StopReporting || !item.Reported.Equal(epoch) {
		t.Errorf("item %+v", item)
	}
}

func TestAdapter_DoReachesAThermometerWithItsValue(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	if err := (&fakeStoreAdapter{parent: f.plugin}).Do("Room", common.ActionSetTemperature, 0, 64); err != nil {
		t.Fatal(err)
	}

	if got := celsiusToFahrenheit(f.reading().Temperature); math.Abs(got-64) > 0.01 {
		t.Errorf("got %v°F", got)
	}
}

func TestConfig_ANameIsOneFakesWhateverTheKind(t *testing.T) {
	conf := NewPluginConfig()
	conf.AddSwitch("Porch")
	conf.AddThermometer("Room")

	for name, add := range map[string]func(){
		"a thermometer named like a switch": func() { conf.AddThermometer("Porch") },
		"a switch named like a thermometer": func() { conf.AddSwitch("Room") },
		"a duplicate thermometer":           func() { conf.AddThermometer("Room") },
		"an empty name":                     func() { conf.AddThermometer("") },
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

	var zero PluginConfig
	zero.AddThermometer("Works without NewPluginConfig")
}

func TestTemperatureConversion(t *testing.T) {
	for _, c := range []struct{ f, c float64 }{{32, 0}, {212, 100}, {-40, -40}, {75, 23.8889}} {
		if got := fahrenheitToCelsius(c.f); !near(got, c.c) {
			t.Errorf("%v°F: %v°C, want %v", c.f, got, c.c)
		}

		if got := celsiusToFahrenheit(float32(c.c)); math.Abs(got-c.f) > 0.01 {
			t.Errorf("%v°C: %v°F, want %v", c.c, got, c.f)
		}
	}

}

func TestThermometer_TheFirstReadingOfOneThatHadNoneIsNotAChangeInTemperature(t *testing.T) {
	f := newFixture(t, &config.PersistentConfigV1{Thermometers: map[string]config.ThermometerStateV1{"Room": {}}})
	f.start()

	_ = f.doTo("Room", common.ActionSetTemperature, 70)

	broadcasts := f.container.broadcastsOf()

	if changes := temperatureChanges(broadcasts); len(changes) != 0 {
		t.Errorf("the first reading was a change: %+v", changes)
	}

	if states := thermometerStates(broadcasts); len(states) != 1 || !states[0].SensorData.HasData {
		t.Errorf("states %+v", states)
	}
}

func TestThermometer_OneThatHasNeverReadAnythingStaysSilentWhenItReports(t *testing.T) {
	f := newFixture(t, &config.PersistentConfigV1{Thermometers: map[string]config.ThermometerStateV1{"Room": {}}})
	f.start()
	f.advance(time.Minute)

	f.container.onPluginGoroutine(f.plugin.report)

	if got := f.reading(); got.HasData || !got.LastUpdateTime.IsZero() {
		t.Errorf("reading %+v", got)
	}
}

func TestStop_WaitsForTheReportingToFinishBeforeSayingItHas(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, nil)
		f.plugin.now = time.Now
		f.container.entered = make(chan struct{}, 1)
		f.start()

		// Catch the reporting goroutine in the middle of handing a report over, when the minute comes.
		f.container.mu.Lock()
		f.container.holdEnqueue = make(chan struct{})
		hold := f.container.holdEnqueue
		f.container.mu.Unlock()

		time.Sleep(time.Minute)
		synctest.Wait()

		select {
		case <-f.container.entered:
		default:
			t.Fatal("the reporting never started")
		}

		var done chan func()

		f.container.onPluginGoroutine(func() { done = f.plugin.Stop() })
		synctest.Wait()

		select {
		case <-done:
			t.Fatal("Stop said it had finished while the reporting was still going")
		default:
		}

		close(hold)
		synctest.Wait()

		select {
		case <-done:
		default:
			t.Fatal("Stop never finished")
		}
	})
}
