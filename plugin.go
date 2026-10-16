package fakes

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/config"
	"github.com/avanha/pmaas-plugin-fakes/data"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	httphandler "github.com/avanha/pmaas-plugin-fakes/internal/http"
	spi "github.com/avanha/pmaas-spi"
	"github.com/avanha/pmaas-spi/alert"
	"github.com/avanha/pmaas-spi/clock"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
	"github.com/avanha/pmaas-spi/events"
)

// reportEvery is how often a fake thermometer that is reporting reports what it reads, as a real one does, so that its
// reading doesn't go stale.
const reportEvery = time.Minute

type Plugin interface {
	spi.IPMAASPlugin
}

// plugin registers fake devices as the entities real ones would be, and shows them on a page, where what PMAAS has
// done to them can be seen and they can be changed.
type plugin struct {
	config      PluginConfig
	container   spi.IPMAASContainer
	httpHandler *httphandler.Handler

	// Owned by the plugin goroutine, once it has started.
	switches     []*fakeSwitch
	thermometers []*fakeThermometer
	doorSensors  []*fakeDoorSensor
	hosts        []*fakeHost
	switchByName map[string]*fakeSwitch
	thermoByName map[string]*fakeThermometer
	doorByName   map[string]*fakeDoorSensor
	hostByName   map[string]*fakeHost

	clockReceiver           int
	clockReceiverRegistered bool

	// reporting is the goroutine that has the thermometers report regularly.
	cancelReporting context.CancelFunc
	reportingWg     sync.WaitGroup

	now         func() time.Time
	reportEvery time.Duration
}

// Force implementation of spi.IPMAASPlugin
var _ spi.IPMAASPlugin = (*plugin)(nil)

func NewPlugin(config PluginConfig) Plugin {
	return &plugin{
		config:       config,
		httpHandler:  httphandler.NewHandler(),
		switchByName: make(map[string]*fakeSwitch),
		thermoByName: make(map[string]*fakeThermometer),
		doorByName:   make(map[string]*fakeDoorSensor),
		hostByName:   make(map[string]*fakeHost),
		now:          time.Now,
		reportEvery:  reportEvery,
	}
}

func (p *plugin) ShortName() string {
	return "fakes"
}

func (p *plugin) Init(container spi.IPMAASContainer) {
	p.container = container
	p.httpHandler.Init(container, &fakeStoreAdapter{parent: p})
}

func (p *plugin) Start() {
	fmt.Printf("%T Starting with %d fake switch(es), %d thermometer(s), %d door sensor(s) and %d host(s)...\n",
		p, len(p.config.switches), len(p.config.thermometers), len(p.config.doorSensors), len(p.config.hosts))

	persisted := p.loadPersistedState()

	for _, handle := range p.config.switches {
		p.startSwitch(handle, persisted.Switches[handle.name])
	}

	for _, handle := range p.config.thermometers {
		saved, found := persisted.Thermometers[handle.name]
		p.startThermometer(handle, saved, found)
	}

	for _, handle := range p.config.doorSensors {
		saved, found := persisted.DoorSensors[handle.name]
		p.startDoorSensor(handle, saved, found)
	}

	for _, handle := range p.config.hosts {
		p.startHost(handle, persisted.Hosts[handle.name])
	}

	// A fake that has never been run before is saved as it starts, so there is a file to look at.
	p.persist()

	p.startReporting()
	p.checkOfflineAlerts()

	handle, err := p.container.RegisterEventReceiver(isMinuteChanged, p.onClockEvent)
	if err != nil {
		fmt.Printf("%T unable to register for clock events, so offline alerts will only follow changes: %v\n", p, err)

		return
	}

	p.clockReceiver, p.clockReceiverRegistered = handle, true
}

// startSwitch makes a fake switch the way it was left, or new if it never was, and registers it.
func (p *plugin) startSwitch(handle *SwitchHandle, saved config.SwitchStateV1) {
	s := restoredSwitch(p, handle.name, saved, p.now())

	id, err := p.container.RegisterEntity(
		handle.name, device.ISwitchType, handle.name,
		func() (any, error) { return s.getStub(p.container), nil })
	if err != nil {
		fmt.Printf("%T unable to register the fake switch %q: %v\n", p, handle.name, err)
		return
	}

	s.id = id
	s.offlineWatch = handle.offline.Watch(p.ShortName(), "offline:"+handle.name, handle.name)
	p.switches = append(p.switches, s)
	p.switchByName[handle.name] = s

	event := spicommon.StubAvailableEvent[device.ISwitch]{EntityEvent: s.entityEvent(), EntityStub: s.getStub(p.container)}
	if err := handle.stub.Provide(p.container, event); err != nil {
		fmt.Printf("%T %v\n", p, err)
	}
}

// startThermometer makes a fake thermometer the way it was left, or new if it never was, and registers it.
func (p *plugin) startThermometer(handle *ThermometerHandle, saved config.ThermometerStateV1, found bool) {
	t := restoredThermometer(p, handle.name, saved, found, p.now())

	id, err := p.container.RegisterEntity(
		handle.name, thermometerType, handle.name,
		func() (any, error) { return t.getStub(p.container), nil })
	if err != nil {
		fmt.Printf("%T unable to register the fake thermometer %q: %v\n", p, handle.name, err)
		return
	}

	t.id = id
	p.thermometers = append(p.thermometers, t)
	p.thermoByName[handle.name] = t

	event := spicommon.StubAvailableEvent[environment.IWirelessThermometer]{
		EntityEvent: t.entityEvent(),
		EntityStub:  t.getStub(p.container),
	}
	if err := handle.stub.Provide(p.container, event); err != nil {
		fmt.Printf("%T %v\n", p, err)
	}
}

// startDoorSensor makes a fake door sensor the way it was left, or new if it never was, and registers it.
func (p *plugin) startDoorSensor(handle *DoorSensorHandle, saved config.DoorSensorStateV1, found bool) {
	d := restoredDoorSensor(p, handle.name, saved, found, p.now())

	id, err := p.container.RegisterEntity(
		handle.name, device.IDoorSensorType, handle.name,
		func() (any, error) { return d.getStub(p.container), nil })
	if err != nil {
		fmt.Printf("%T unable to register the fake door sensor %q: %v\n", p, handle.name, err)
		return
	}

	d.id = id
	d.offlineWatch = handle.offline.Watch(p.ShortName(), "offline:"+handle.name, handle.name)
	p.doorSensors = append(p.doorSensors, d)
	p.doorByName[handle.name] = d

	event := spicommon.StubAvailableEvent[device.IDoorSensor]{EntityEvent: d.entityEvent(), EntityStub: d.getStub(p.container)}
	if err := handle.stub.Provide(p.container, event); err != nil {
		fmt.Printf("%T %v\n", p, err)
	}
}

// startHost makes a fake host the way it was left, or up if it never was. A host is not an entity: netmon registers the
// real one, and asks the handle whether it answers.
func (p *plugin) startHost(handle *HostHandle, saved config.HostStateV1) {
	h := &fakeHost{plugin: p, name: handle.name, handle: handle, down: saved.Down, since: saved.Since}
	handle.down.Store(saved.Down)

	if !saved.Down && h.since.IsZero() {
		h.since = p.now()
	}

	p.hosts = append(p.hosts, h)
	p.hostByName[handle.name] = h
}

// startReporting has the thermometers report regularly. Without it a reading would go stale even though nothing
// was wrong, as it would with a real sensor that stopped talking.
func (p *plugin) startReporting() {
	if len(p.thermometers) == 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancelReporting = cancel

	p.reportingWg.Go(func() {
		ticker := time.NewTicker(p.reportEvery)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// If the plugin has stopped there is nothing to report to, and the tick is dropped.
				_ = p.container.EnqueueOnPluginGoRoutine(p.report)
			}
		}
	})
}

// report has every thermometer that is reporting report. Runs on the plugin goroutine.
func (p *plugin) report() {
	for _, t := range p.thermometers {
		t.refresh()
	}
}

// onClockEvent looks again at the fakes that are offline each minute, since an alert that waits for a time comes due
// without anything changing. Runs on the plugin goroutine.
func (p *plugin) onClockEvent(eventInfo *events.EventInfo) error {
	if _, ok := eventInfo.Event.(clock.MinuteChangedEvent); ok {
		p.checkOfflineAlerts()
	}

	return nil
}

func isMinuteChanged(eventInfo *events.EventInfo) bool {
	_, ok := eventInfo.Event.(clock.MinuteChangedEvent)

	return ok
}

// checkOfflineAlerts looks at every fake that has an alert for going offline. Runs on the plugin goroutine.
func (p *plugin) checkOfflineAlerts() {
	for _, s := range p.switches {
		s.checkOffline()
	}

	for _, d := range p.doorSensors {
		d.checkOffline()
	}
}

// checkOffline raises, escalates or clears one fake's alert about being offline. Runs on the plugin goroutine.
func (p *plugin) checkOffline(watch *alert.OfflineWatch, connectivity environment.Connectivity, offlineSince time.Time) {
	if err := watch.Check(p.container, connectivity, offlineSince, p.now()); err != nil {
		fmt.Printf("%T unable to update an offline alert: %v\n", p, err)
	}
}

func (p *plugin) Stop() chan func() {
	fmt.Printf("%T Stopping...\n", p)

	if p.cancelReporting != nil {
		p.cancelReporting()
		p.cancelReporting = nil
	}

	if p.clockReceiverRegistered {
		p.clockReceiverRegistered = false

		if err := p.container.DeregisterEventReceiver(p.clockReceiver); err != nil {
			fmt.Printf("%T unable to deregister the clock event receiver: %v\n", p, err)
		}
	}

	switchHandles := make(map[string]*SwitchHandle, len(p.config.switches))
	for _, handle := range p.config.switches {
		switchHandles[handle.name] = handle
	}

	thermometerHandles := make(map[string]*ThermometerHandle, len(p.config.thermometers))
	for _, handle := range p.config.thermometers {
		thermometerHandles[handle.name] = handle
	}

	for _, s := range p.switches {
		// Before the stub is closed, so that nobody is handed one that is no use.
		switchHandles[s.name].stub.Withdraw()

		if err := p.container.DeregisterEntity(s.id); err != nil {
			fmt.Printf("%T unable to deregister the fake switch %q: %v\n", p, s.name, err)
		}

		s.closeStub()
	}

	for _, t := range p.thermometers {
		thermometerHandles[t.name].stub.Withdraw()

		if err := p.container.DeregisterEntity(t.id); err != nil {
			fmt.Printf("%T unable to deregister the fake thermometer %q: %v\n", p, t.name, err)
		}

		t.closeStub()
	}

	doorHandles := make(map[string]*DoorSensorHandle, len(p.config.doorSensors))
	for _, handle := range p.config.doorSensors {
		doorHandles[handle.name] = handle
	}

	for _, d := range p.doorSensors {
		doorHandles[d.name].stub.Withdraw()

		if err := p.container.DeregisterEntity(d.id); err != nil {
			fmt.Printf("%T unable to deregister the fake door sensor %q: %v\n", p, d.name, err)
		}

		d.closeStub()
	}

	p.switches, p.switchByName = nil, make(map[string]*fakeSwitch)
	p.thermometers, p.thermoByName = nil, make(map[string]*fakeThermometer)
	p.doorSensors, p.doorByName = nil, make(map[string]*fakeDoorSensor)
	p.hosts, p.hostByName = nil, make(map[string]*fakeHost)

	// The goroutine that has them report is told to stop above, and finishes promptly; wait for it away from here, which
	// is the plugin's own goroutine.
	done := make(chan func())

	go func() {
		defer close(done)

		p.reportingWg.Wait()
	}()

	return done
}

func (p *plugin) broadcast(id string, event any) {
	if err := p.container.BroadcastEvent(id, event); err != nil {
		fmt.Printf("%T unable to broadcast %T for %s: %v\n", p, event, id, err)
	}
}

// do does something to a fake from the page. Runs on the plugin goroutine.
func (p *plugin) do(name string, action common.Action, count int, value float64) error {
	if s, ok := p.switchByName[name]; ok {
		return s.do(action, count)
	}

	if t, ok := p.thermoByName[name]; ok {
		return t.do(action, value)
	}

	if d, ok := p.doorByName[name]; ok {
		return d.do(action)
	}

	if h, ok := p.hostByName[name]; ok {
		return h.do(action)
	}

	return common.Refuse("there is no fake %q", name)
}

// getFakes builds what the page shows. Runs on the plugin goroutine.
func (p *plugin) getFakes() data.Fakes {
	result := data.Fakes{
		Switches:     make([]data.Switch, len(p.switches)),
		Thermometers: make([]data.Thermometer, len(p.thermometers)),
		DoorSensors:  make([]data.DoorSensor, len(p.doorSensors)),
		Hosts:        make([]data.Host, len(p.hosts)),
	}

	for i, s := range p.switches {
		view := data.Switch{
			Name:           s.name,
			State:          s.data.State.String(),
			StateClass:     strings.ToLower(s.data.State.String()),
			Offline:        s.data.Connectivity == environment.ConnectivityOffline,
			LastChangeTime: s.data.LastChangeTime,
			LastUpdateTime: s.data.LastUpdateTime,
			CommandedAt:    s.commandedAt,
			IgnoreCommands: s.ignoreCommands,
			FailNext:       s.failNext,
			History:        make([]data.Command, len(s.history)),
		}

		if s.commanded != device.SwitchStateUnknown {
			view.Commanded = s.commanded.String()
		}

		for j, c := range s.history {
			view.History[j] = data.Command{At: c.at, State: c.state.String(), Accepted: c.accepted, Result: c.result}
		}

		result.Switches[i] = view
	}

	for i, t := range p.thermometers {
		reading := t.data.SensorData

		result.Thermometers[i] = data.Thermometer{
			Name:          t.name,
			HasData:       reading.HasData,
			Fahrenheit:    float32(celsiusToFahrenheit(reading.Temperature)),
			Celsius:       reading.Temperature,
			Reported:      reading.LastUpdateTime,
			StopReporting: t.stopReporting,
		}
	}

	for i, d := range p.doorSensors {
		result.DoorSensors[i] = data.DoorSensor{
			Name:           d.name,
			Position:       d.data.Position.String(),
			PositionClass:  strings.ToLower(d.data.Position.String()),
			Offline:        d.data.Connectivity == environment.ConnectivityOffline,
			LastChangeTime: d.data.LastChangeTime,
			LastUpdateTime: d.data.LastUpdateTime,
		}
	}

	for i, h := range p.hosts {
		result.Hosts[i] = data.Host{Name: h.name, Down: h.down, Since: h.since}
	}

	return result
}

// persist keeps the fakes as they are. Runs on the plugin goroutine.
func (p *plugin) persist() {
	persisted := config.PersistentConfigV1{
		Switches:     make(map[string]config.SwitchStateV1, len(p.switches)),
		Thermometers: make(map[string]config.ThermometerStateV1, len(p.thermometers)),
		DoorSensors:  make(map[string]config.DoorSensorStateV1, len(p.doorSensors)),
		Hosts:        make(map[string]config.HostStateV1, len(p.hosts)),
	}

	for _, s := range p.switches {
		persisted.Switches[s.name] = s.persisted()
	}

	for _, t := range p.thermometers {
		persisted.Thermometers[t.name] = t.persisted()
	}

	for _, d := range p.doorSensors {
		persisted.DoorSensors[d.name] = d.persisted()
	}

	for _, h := range p.hosts {
		persisted.Hosts[h.name] = config.HostStateV1{Down: h.down, Since: h.since}
	}

	if err := p.container.SaveConfig(persisted); err != nil {
		fmt.Printf("%T unable to keep the fakes: %v\n", p, err)
	}
}

// loadPersistedState is what was kept. Nothing, if there was nothing, or it can't be read: the fakes then start as new.
func (p *plugin) loadPersistedState() config.PersistentConfigV1 {
	loaded, err := p.container.LoadConfig(func(typeName string) any {
		v1Type := reflect.TypeFor[config.PersistentConfigV1]()

		if typeName == v1Type.PkgPath()+"/"+v1Type.Name() {
			return &config.PersistentConfigV1{}
		}

		return nil
	})

	if err != nil || loaded == nil {
		return config.PersistentConfigV1{}
	}

	if persisted, ok := loaded.(*config.PersistentConfigV1); ok {
		return *persisted
	}

	return config.PersistentConfigV1{}
}
