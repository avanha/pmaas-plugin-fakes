package fakes

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/config"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	spi "github.com/avanha/pmaas-spi"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
	"github.com/avanha/pmaas-spi/events"
)

var epoch = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// fakeContainer runs "the plugin goroutine" as a goroutine of its own, so that stubs hop where they would.
type fakeContainer struct {
	spi.IPMAASContainer

	work chan func()

	mu            sync.Mutex
	registrations []registration
	deregistered  []string
	broadcasts    []any
	saved         []config.PersistentConfigV1
	persisted     *config.PersistentConfigV1
	serverBatches [][]func()

	// receiver is the clock event receiver the plugin registered, if it has; receiversDeregistered are the handles it gave back.
	receiver              events.EventReceiver
	receiversDeregistered []int

	// holdEnqueue, when set, stops EnqueueOnPluginGoRoutine until it is closed, and entered is told when a caller is held.
	holdEnqueue chan struct{}
	entered     chan struct{}

	routes        map[string]*spi.HttpHandlerOptions
	jsonRoutes    []string
	rendererTypes []reflect.Type
}

type registration struct {
	id          string
	uniqueData  string
	entityType  reflect.Type
	name        string
	stubFactory spi.EntityStubFactoryFunc
}

func newFakeContainer(t *testing.T) *fakeContainer {
	t.Helper()

	c := &fakeContainer{work: make(chan func(), 64)}

	go func() {
		for f := range c.work {
			f()
		}
	}()

	t.Cleanup(func() { close(c.work) })

	return c
}

func (c *fakeContainer) EnqueueOnPluginGoRoutine(f func()) error {
	c.mu.Lock()
	hold, entered := c.holdEnqueue, c.entered
	c.mu.Unlock()

	// A test can hold callers here, to have a goroutine caught in the middle of one.
	if hold != nil {
		select {
		case entered <- struct{}{}:
		default:
		}

		<-hold
	}

	c.work <- f

	return nil
}

func (c *fakeContainer) EnqueueOnServerGoRoutine(invocations []func()) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.serverBatches = append(c.serverBatches, invocations)

	return nil
}

// onPluginGoroutine runs f where the plugin's own code does, and waits for it.
func (c *fakeContainer) onPluginGoroutine(f func()) {
	done := make(chan struct{})
	c.work <- func() { f(); close(done) }
	<-done
}

func (c *fakeContainer) RegisterEventReceiver(predicate events.EventPredicate, receiver events.EventReceiver) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.receiver = receiver

	return 7, nil
}

func (c *fakeContainer) DeregisterEventReceiver(handle int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.receiver = nil
	c.receiversDeregistered = append(c.receiversDeregistered, handle)

	return nil
}

func (c *fakeContainer) ProvideContentFS(fs.FS, string) {}
func (c *fakeContainer) EnableStaticContent(string)     {}

func (c *fakeContainer) AddRouteWithOptions(path string, _ http.HandlerFunc, options *spi.HttpHandlerOptions) {
	if c.routes == nil {
		c.routes = make(map[string]*spi.HttpHandlerOptions)
	}

	c.routes[path] = options
}

func (c *fakeContainer) AddJsonRoute(path string, _ spi.RequestObjectFactoryFunc, _ spi.JsonHandlerFunc) {
	c.jsonRoutes = append(c.jsonRoutes, path)
}

func (c *fakeContainer) RegisterEntityRenderer(entityType reflect.Type, _ spi.EntityRendererFactory) {
	c.rendererTypes = append(c.rendererTypes, entityType)
}

func (c *fakeContainer) RegisterEntity(uniqueData string, entityType reflect.Type, name string, factory spi.EntityStubFactoryFunc) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := "fakes_" + uniqueData
	c.registrations = append(c.registrations, registration{id, uniqueData, entityType, name, factory})

	return id, nil
}

func (c *fakeContainer) DeregisterEntity(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deregistered = append(c.deregistered, id)

	return nil
}

func (c *fakeContainer) BroadcastEvent(_ string, event any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.broadcasts = append(c.broadcasts, event)

	return nil
}

func (c *fakeContainer) SaveConfig(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.saved = append(c.saved, value.(config.PersistentConfigV1))

	return nil
}

// LoadConfig hands back what was "saved" the way the real one does: by filling in what the plugin's factory makes for
// the type's name.
func (c *fakeContainer) LoadConfig(factory func(typeName string) any) (any, error) {
	if c.persisted == nil {
		return nil, errors.New("nothing saved")
	}

	persistedType := reflect.TypeFor[config.PersistentConfigV1]()

	target := factory(persistedType.PkgPath() + "/" + persistedType.Name())
	if target == nil {
		return nil, errors.New("not a type the plugin knows")
	}

	encoded, err := json.Marshal(c.persisted)
	if err != nil {
		return nil, err
	}

	return target, json.Unmarshal(encoded, target)
}

func (c *fakeContainer) ClosedCallbackChannel() chan func() {
	ch := make(chan func())
	close(ch)

	return ch
}

func (c *fakeContainer) broadcastsOf() []any {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]any(nil), c.broadcasts...)
}

func (c *fakeContainer) lastSaved(t *testing.T) config.PersistentConfigV1 {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.saved) == 0 {
		t.Fatal("nothing was saved")
	}

	return c.saved[len(c.saved)-1]
}

func (c *fakeContainer) saveCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.saved)
}

// fixture is a started plugin with one fake switch, whose clock is the test's.
type fixture struct {
	t         *testing.T
	container *fakeContainer
	plugin    *plugin
	handle    *SwitchHandle
	thermo    *ThermometerHandle
	door      *DoorSensorHandle
	host      *HostHandle
	clock     time.Time
}

func newFixture(t *testing.T, persisted *config.PersistentConfigV1) *fixture {
	t.Helper()

	f := &fixture{t: t, container: newFakeContainer(t), clock: epoch}
	f.container.persisted = persisted

	conf := NewPluginConfig()
	f.handle = conf.AddSwitch("Porch")
	f.thermo = conf.AddThermometer("Room")
	f.door = conf.AddDoorSensor("Garage door")
	f.host = conf.AddHost("Router")

	f.plugin = NewPlugin(conf).(*plugin)
	f.plugin.now = func() time.Time { return f.clock }

	return f
}

func (f *fixture) start() {
	f.t.Helper()

	f.container.onPluginGoroutine(func() {
		f.plugin.Init(f.container)
		f.plugin.Start()
	})
}

func (f *fixture) advance(d time.Duration) { f.clock = f.clock.Add(d) }

// do does something to the fake from the page, where the plugin's code does, and returns what it said.
func (f *fixture) do(action common.Action, count int) error {
	f.t.Helper()

	var err error

	f.container.onPluginGoroutine(func() { err = f.plugin.do("Porch", action, count, 0) })

	return err
}

// switchData is what the entity reports, read the way anyone but the plugin does: through the stub.
func (f *fixture) switchData() device.Switch {
	f.t.Helper()

	return f.handle.GetSwitchData()
}

func changes(broadcasts []any) []device.SwitchStateChangedEvent {
	var result []device.SwitchStateChangedEvent

	for _, b := range broadcasts {
		if c, ok := b.(device.SwitchStateChangedEvent); ok {
			result = append(result, c)
		}
	}

	return result
}

func states(broadcasts []any) []device.Switch {
	var result []device.Switch

	for _, b := range broadcasts {
		if c, ok := b.(events.EntityStateChangedEvent); ok {
			result = append(result, c.NewState.(device.Switch))
		}
	}

	return result
}

func TestInit_ProvidesThePageAndTheRouteToChangeThings(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	options, ok := f.container.routes[""]
	if !ok || options.MenuLabel != "Fakes" || !options.SupportsXsrfValidation {
		t.Errorf("list route: %+v", options)
	}

	if !reflect.DeepEqual(f.container.jsonRoutes, []string{"control"}) {
		t.Errorf("json routes: %v", f.container.jsonRoutes)
	}

	if f.plugin.ShortName() != "fakes" {
		t.Errorf("name %q", f.plugin.ShortName())
	}
}

func TestStart_RegistersTheSwitchAsAnEntityWithAStateOfItsOwn(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	if len(f.container.registrations) != 3 {
		t.Fatalf("registrations: %+v", f.container.registrations)
	}

	r := f.container.registrations[0]
	if r.entityType != device.ISwitchType || r.name != "Porch" || r.uniqueData != "Porch" {
		t.Errorf("registration: %+v", r)
	}

	// The stub the container is given is the same thing that the assembly's handle goes through.
	stub, err := r.stubFactory()
	if err != nil {
		t.Fatal(err)
	}

	data := stub.(device.ISwitch).GetSwitchData()
	if data.Name != "Porch" || !data.HasData || data.State != device.SwitchStateOff ||
		data.Connectivity != environment.ConnectivityOnline || !data.OnlineSince.Equal(epoch) {
		t.Errorf("a new fake: %+v", data)
	}

	if saved := f.container.lastSaved(t).Switches["Porch"]; saved.State != "off" || !saved.HasData {
		t.Errorf("not kept: %+v", saved)
	}
}

func TestHandle_IsASwitchBeforeThePluginHasStartedAndAfterItHasStopped(t *testing.T) {
	f := newFixture(t, nil)

	if got := f.handle.GetSwitchData(); got.Name != "Porch" || got.HasData {
		t.Errorf("before: %+v", got)
	}

	if err := f.handle.SetState(device.SwitchStateOn); !errors.Is(err, spicommon.ErrStubNotAvailable) {
		t.Errorf("before: %v", err)
	}

	f.start()

	if err := f.handle.SetState(device.SwitchStateOn); err != nil {
		t.Fatalf("while running: %v", err)
	}

	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	if !reflect.DeepEqual(f.container.deregistered, []string{"fakes_Porch", "fakes_Room", "fakes_Garage door"}) {
		t.Errorf("deregistered %v", f.container.deregistered)
	}

	if got := f.handle.GetSwitchData(); got.HasData {
		t.Errorf("after: %+v", got)
	}

	if err := f.handle.SetState(device.SwitchStateOff); !errors.Is(err, spicommon.ErrStubNotAvailable) {
		t.Errorf("after: %v", err)
	}
}

func TestStop_AStubTakenBeforeItFailsFastRatherThanReachingAnEntityThatIsGone(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	stub, err := f.container.registrations[0].stubFactory()
	if err != nil {
		t.Fatal(err)
	}

	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	defer func() {
		if recover() == nil {
			t.Error("a stub of a switch that has been deregistered still worked")
		}
	}()

	stub.(device.ISwitch).GetSwitchData()
}

func TestSetState_AcceptedCommandChangesWhatTheDeviceReportsAndSaysSo(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	f.advance(time.Minute)

	if err := f.handle.SetState(device.SwitchStateOn); err != nil {
		t.Fatal(err)
	}

	data := f.switchData()
	if data.State != device.SwitchStateOn || !data.LastChangeTime.Equal(f.clock) || !data.LastUpdateTime.Equal(f.clock) {
		t.Errorf("data %+v", data)
	}

	broadcasts := f.container.broadcastsOf()
	if got := changes(broadcasts); len(got) != 1 || got[0].NewState != device.SwitchStateOn ||
		got[0].OldState != device.SwitchStateOff || got[0].Name != "Porch" || got[0].EntityType != device.ISwitchType {
		t.Errorf("changes %+v", got)
	}

	if got := states(broadcasts); len(got) != 1 || got[0].State != device.SwitchStateOn {
		t.Errorf("states %+v", got)
	}

	saved := f.container.lastSaved(t).Switches["Porch"]
	if saved.State != "on" || saved.Commanded != "on" || !saved.CommandedAt.Equal(f.clock) ||
		len(saved.History) != 1 || !saved.History[0].Accepted || saved.History[0].State != "on" {
		t.Errorf("kept %+v", saved)
	}
}

func TestSetState_AskingForTheStateItIsAlreadyInIsAcceptedAndNotNews(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	before := len(f.container.broadcastsOf())

	if err := f.handle.SetState(device.SwitchStateOff); err != nil {
		t.Fatal(err)
	}

	if len(f.container.broadcastsOf()) != before {
		t.Error("an unchanged state was broadcast")
	}

	// But it is a command that was sent, so it is remembered.
	if saved := f.container.lastSaved(t).Switches["Porch"]; saved.Commanded != "off" || len(saved.History) != 1 {
		t.Errorf("kept %+v", saved)
	}
}

func TestSetState_AStateThatCanNotBeAskedForIsRefused(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	err := f.handle.SetState(device.SwitchStateUnknown)
	if !errors.Is(err, device.ErrInvalidSwitchState) {
		t.Errorf("got %v", err)
	}

	if saved := f.container.lastSaved(t).Switches["Porch"]; len(saved.History) != 0 {
		t.Errorf("a command that never reached the device was recorded: %+v", saved.History)
	}
}

func TestOffline_RefusesCommandsAndKeepsItsState(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	_ = f.handle.SetState(device.SwitchStateOn)

	f.advance(time.Minute)

	if err := f.do(common.ActionOffline, 0); err != nil {
		t.Fatal(err)
	}

	data := f.switchData()
	if data.Connectivity != environment.ConnectivityOffline || !data.OfflineSince.Equal(f.clock) || data.State != device.SwitchStateOn {
		t.Errorf("offline: %+v", data)
	}

	err := f.handle.SetState(device.SwitchStateOff)
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("a command to an offline device: %v", err)
	}

	if f.switchData().State != device.SwitchStateOn {
		t.Error("an offline device changed state")
	}

	saved := f.container.lastSaved(t).Switches["Porch"]
	if !saved.Offline || len(saved.History) != 2 || saved.History[0].Accepted || !strings.Contains(saved.History[0].Result, "offline") {
		t.Errorf("kept %+v", saved)
	}

	// And it can't report anything either.
	var refused *common.RequestError
	if err := f.do(common.ActionReportOff, 0); !errors.As(err, &refused) {
		t.Errorf("pressing the button of an offline device: %v", err)
	}

	f.advance(time.Minute)

	if err := f.do(common.ActionOnline, 0); err != nil {
		t.Fatal(err)
	}

	if data := f.switchData(); data.Connectivity != environment.ConnectivityOnline || !data.OnlineSince.Equal(f.clock) {
		t.Errorf("online: %+v", data)
	}

	if err := f.handle.SetState(device.SwitchStateOff); err != nil {
		t.Errorf("a command to a device that is back: %v", err)
	}
}

func TestOffline_IsNotNewsWhenItAlreadyWas(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	_ = f.do(common.ActionOnline, 0)

	if got := states(f.container.broadcastsOf()); len(got) != 0 {
		t.Errorf("bringing an online device online was broadcast: %+v", got)
	}

	_ = f.do(common.ActionOffline, 0)
	before := len(f.container.broadcastsOf())
	_ = f.do(common.ActionOffline, 0)

	if len(f.container.broadcastsOf()) != before {
		t.Error("taking an offline device offline was broadcast")
	}
}

func TestFailNext_FailsThatManyCommandsThenWorks(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	if err := f.do(common.ActionFailNext, 2); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := f.handle.SetState(device.SwitchStateOn); err == nil {
			t.Fatal("a command that was to fail worked")
		}

		if f.switchData().State != device.SwitchStateOff {
			t.Fatal("a failed command changed the state")
		}
	}

	if err := f.handle.SetState(device.SwitchStateOn); err != nil {
		t.Fatalf("the third: %v", err)
	}

	saved := f.container.lastSaved(t).Switches["Porch"]
	if saved.FailNext != 0 || len(saved.History) != 3 || !saved.History[0].Accepted || saved.History[1].Accepted ||
		saved.Commanded != "on" {
		t.Errorf("kept %+v", saved)
	}
}

func TestFailNext_CanBeCancelledAndIsChecked(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	_ = f.do(common.ActionFailNext, 5)
	_ = f.do(common.ActionFailNext, 0)

	if err := f.handle.SetState(device.SwitchStateOn); err != nil {
		t.Errorf("after cancelling: %v", err)
	}

	var refused *common.RequestError
	for _, count := range []int{-1, maxFailNext + 1} {
		if err := f.do(common.ActionFailNext, count); !errors.As(err, &refused) {
			t.Errorf("%d: %v", count, err)
		}
	}

	if err := f.do(common.ActionFailNext, maxFailNext); err != nil {
		t.Errorf("the most: %v", err)
	}
}

func TestIgnoreCommands_AcceptsThemAndDoesNothing(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	if err := f.do(common.ActionIgnoreCommands, 0); err != nil {
		t.Fatal(err)
	}

	if err := f.handle.SetState(device.SwitchStateOn); err != nil {
		t.Fatalf("an ignored command is still accepted: %v", err)
	}

	if f.switchData().State != device.SwitchStateOff {
		t.Error("the device acted on a command it was told to ignore")
	}

	saved := f.container.lastSaved(t).Switches["Porch"]
	if !saved.IgnoreCommands || saved.Commanded != "on" || saved.State != "off" {
		t.Errorf("kept %+v", saved)
	}

	_ = f.do(common.ActionFollowCommands, 0)
	_ = f.handle.SetState(device.SwitchStateOn)

	if f.switchData().State != device.SwitchStateOn {
		t.Error("the device doesn't act on commands again")
	}
}

func TestReport_TheDeviceChangingByItselfIsNewsLikeAnyChange(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	f.advance(time.Minute)

	if err := f.do(common.ActionReportOn, 0); err != nil {
		t.Fatal(err)
	}

	if f.switchData().State != device.SwitchStateOn {
		t.Fatal("not on")
	}

	if got := changes(f.container.broadcastsOf()); len(got) != 1 || got[0].NewState != device.SwitchStateOn {
		t.Errorf("changes %+v", got)
	}

	// PMAAS never asked.
	if saved := f.container.lastSaved(t).Switches["Porch"]; saved.Commanded != "unknown" || len(saved.History) != 0 {
		t.Errorf("kept %+v", saved)
	}

	before := len(f.container.broadcastsOf())
	_ = f.do(common.ActionReportOn, 0)

	if len(f.container.broadcastsOf()) != before {
		t.Error("a repeat of the same state was broadcast")
	}

	_ = f.do(common.ActionReportOff, 0)

	if f.switchData().State != device.SwitchStateOff {
		t.Error("not off")
	}
}

func TestReport_TheFirstReportOfASwitchThatHadNeverReportedIsNotAChange(t *testing.T) {
	f := newFixture(t, &config.PersistentConfigV1{Switches: map[string]config.SwitchStateV1{
		"Porch": {State: "unknown", HasData: false, OnlineSince: epoch},
	}})
	f.start()

	if f.switchData().HasData {
		t.Fatal("restored with data it didn't have")
	}

	_ = f.do(common.ActionReportOn, 0)

	if got := changes(f.container.broadcastsOf()); len(got) != 0 {
		t.Errorf("the first report was a change: %+v", got)
	}

	if got := states(f.container.broadcastsOf()); len(got) != 1 {
		t.Errorf("states %+v", got)
	}

	if data := f.switchData(); !data.HasData || data.State != device.SwitchStateOn || !data.LastChangeTime.IsZero() {
		t.Errorf("data %+v", data)
	}
}

func TestHistory_IsBoundedNewestFirstAndCanBeCleared(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	for i := range maxHistory + 5 {
		f.advance(time.Second)

		state := device.SwitchStateOn
		if i%2 == 1 {
			state = device.SwitchStateOff
		}

		_ = f.handle.SetState(state)
	}

	saved := f.container.lastSaved(t).Switches["Porch"]
	if len(saved.History) != maxHistory || !saved.History[0].At.After(saved.History[1].At) {
		t.Fatalf("history of %d, first two %v %v", len(saved.History), saved.History[0].At, saved.History[1].At)
	}

	if !saved.History[0].At.Equal(f.clock) {
		t.Errorf("newest is %v, want %v", saved.History[0].At, f.clock)
	}

	if err := f.do(common.ActionClearHistory, 0); err != nil {
		t.Fatal(err)
	}

	saved = f.container.lastSaved(t).Switches["Porch"]
	if len(saved.History) != 0 || saved.Commanded != "unknown" || !saved.CommandedAt.IsZero() {
		t.Errorf("after clearing %+v", saved)
	}
}

func TestDo_RefusesWhatCanNotBeDone(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	var refused *common.RequestError

	if err := f.do("dance", 0); !errors.As(err, &refused) {
		t.Errorf("an unknown action: %v", err)
	}

	var err error

	f.container.onPluginGoroutine(func() { err = f.plugin.do("Nowhere", common.ActionOffline, 0, 0) })

	if !errors.As(err, &refused) {
		t.Errorf("an unknown switch: %v", err)
	}
}

func TestRestart_TheFakeComesBackAsItWasLeft(t *testing.T) {
	first := newFixture(t, nil)
	first.start()
	first.advance(time.Minute)
	_ = first.handle.SetState(device.SwitchStateOn)
	_ = first.do(common.ActionFailNext, 3)
	_ = first.do(common.ActionIgnoreCommands, 0)
	_ = first.do(common.ActionOffline, 0)

	left := first.container.lastSaved(t)

	second := newFixture(t, &left)
	second.clock = epoch.Add(time.Hour)
	second.start()

	data := second.switchData()
	if data.State != device.SwitchStateOn || !data.HasData || data.Connectivity != environment.ConnectivityOffline ||
		!data.LastChangeTime.Equal(epoch.Add(time.Minute)) {
		t.Errorf("data %+v", data)
	}

	got := second.container.lastSaved(t).Switches["Porch"]
	want := left.Switches["Porch"]

	if !reflect.DeepEqual(got, want) {
		t.Errorf("after a restart:\n got  %+v\n want %+v", got, want)
	}
}

func TestRestart_AFileThatIsMissingThingsOrHasWordsItDoesNotKnowIsStillAFake(t *testing.T) {
	f := newFixture(t, &config.PersistentConfigV1{Switches: map[string]config.SwitchStateV1{
		"Porch": {State: "sideways", HasData: true, Commanded: "bananas"},
	}})
	f.start()

	if data := f.switchData(); data.State != device.SwitchStateUnknown || data.Connectivity != environment.ConnectivityOnline {
		t.Errorf("data %+v", data)
	}

	// A fake that isn't in the file at all starts new.
	other := newFixture(t, &config.PersistentConfigV1{Switches: map[string]config.SwitchStateV1{"Elsewhere": {State: "on", HasData: true}}})
	other.start()

	if data := other.switchData(); data.State != device.SwitchStateOff || !data.HasData {
		t.Errorf("data %+v", data)
	}
}

func TestGetSwitches_ShowsTheStateWhatWasAskedAndTheSettings(t *testing.T) {
	f := newFixture(t, nil)
	f.start()
	f.advance(time.Minute)
	_ = f.handle.SetState(device.SwitchStateOn)
	_ = f.do(common.ActionFailNext, 2)
	_ = f.do(common.ActionIgnoreCommands, 0)
	_ = f.handle.SetState(device.SwitchStateOff) // fails, once
	_ = f.do(common.ActionOffline, 0)

	shown, err := (&fakeStoreAdapter{parent: f.plugin}).GetFakes()
	if err != nil {
		t.Fatal(err)
	}

	if len(shown.Switches) != 1 {
		t.Fatalf("shown %+v", shown)
	}

	item := shown.Switches[0]
	if item.Name != "Porch" || item.State != "On" || item.StateClass != "on" || !item.Offline ||
		item.Commanded != "On" || !item.CommandedAt.Equal(epoch.Add(time.Minute)) ||
		!item.IgnoreCommands || item.FailNext != 1 || len(item.History) != 2 ||
		item.History[0].Accepted || item.History[0].State != "Off" || !item.History[1].Accepted {
		t.Errorf("item %+v", item)
	}
}

func TestGetSwitches_NothingAskedYet(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	shown, _ := (&fakeStoreAdapter{parent: f.plugin}).GetFakes()
	if shown.Switches[0].Commanded != "" || len(shown.Switches[0].History) != 0 {
		t.Errorf("item %+v", shown.Switches[0])
	}
}

func TestAdapter_DoReachesThePlugin(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	adapter := &fakeStoreAdapter{parent: f.plugin}

	if err := adapter.Do("Porch", common.ActionReportOn, 0, 0); err != nil {
		t.Fatal(err)
	}

	if f.switchData().State != device.SwitchStateOn {
		t.Error("not on")
	}

	var refused *common.RequestError
	if err := adapter.Do("Nowhere", common.ActionReportOn, 0, 0); !errors.As(err, &refused) {
		t.Errorf("an unknown switch: %v", err)
	}
}

func TestConfig_AddSwitchRefusesWhatCouldNotWork(t *testing.T) {
	conf := NewPluginConfig()
	first := conf.AddSwitch("Porch")

	if first.Name() != "Porch" {
		t.Errorf("name %q", first.Name())
	}

	for name, add := range map[string]func(){
		"duplicate": func() { conf.AddSwitch("Porch") },
		"empty":     func() { conf.AddSwitch("") },
		"blank":     func() { conf.AddSwitch("  ") },
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
	zero.AddSwitch("Works without NewPluginConfig")
}

func TestSwitchStateWords(t *testing.T) {
	for state, word := range map[device.SwitchState]string{
		device.SwitchStateOn: "on", device.SwitchStateOff: "off", device.SwitchStateUnknown: "unknown",
	} {
		if got := switchStateWord(state); got != word {
			t.Errorf("%v: %q", state, got)
		}

		if got := switchStateOf(word); got != state {
			t.Errorf("%q: %v", word, got)
		}
	}

	if switchStateOf("") != device.SwitchStateUnknown {
		t.Error("an empty word is a state")
	}
}
