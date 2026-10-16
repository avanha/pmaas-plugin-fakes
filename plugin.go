package fakes

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/config"
	"github.com/avanha/pmaas-plugin-fakes/data"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	httphandler "github.com/avanha/pmaas-plugin-fakes/internal/http"
	spi "github.com/avanha/pmaas-spi"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

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
	switches []*fakeSwitch
	byName   map[string]*fakeSwitch

	now func() time.Time
}

// Force implementation of spi.IPMAASPlugin
var _ spi.IPMAASPlugin = (*plugin)(nil)

func NewPlugin(config PluginConfig) Plugin {
	return &plugin{config: config, httpHandler: httphandler.NewHandler(), byName: make(map[string]*fakeSwitch), now: time.Now}
}

func (p *plugin) ShortName() string {
	return "fakes"
}

func (p *plugin) Init(container spi.IPMAASContainer) {
	p.container = container
	p.httpHandler.Init(container, &switchStoreAdapter{parent: p})
}

func (p *plugin) Start() {
	fmt.Printf("%T Starting with %d fake switch(es)...\n", p, len(p.config.switches))

	persisted := p.loadPersistedState()

	for _, handle := range p.config.switches {
		p.startSwitch(handle, persisted.Switches[handle.name])
	}

	// A fake that has never been run before is saved as it starts, so there is a file to look at.
	p.persist()
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
	p.switches = append(p.switches, s)
	p.byName[handle.name] = s

	event := spicommon.StubAvailableEvent[device.ISwitch]{EntityEvent: s.entityEvent(), EntityStub: s.getStub(p.container)}
	if err := handle.stub.Provide(p.container, event); err != nil {
		fmt.Printf("%T %v\n", p, err)
	}
}

func (p *plugin) Stop() chan func() {
	fmt.Printf("%T Stopping...\n", p)

	handles := make(map[string]*SwitchHandle, len(p.config.switches))
	for _, handle := range p.config.switches {
		handles[handle.name] = handle
	}

	for _, s := range p.switches {
		// Before the stub is closed, so that nobody is handed one that is no use.
		handles[s.name].stub.Withdraw()

		if err := p.container.DeregisterEntity(s.id); err != nil {
			fmt.Printf("%T unable to deregister the fake switch %q: %v\n", p, s.name, err)
		}

		s.closeStub()
	}

	p.switches, p.byName = nil, make(map[string]*fakeSwitch)

	return p.container.ClosedCallbackChannel()
}

func (p *plugin) broadcast(id string, event any) {
	if err := p.container.BroadcastEvent(id, event); err != nil {
		fmt.Printf("%T unable to broadcast %T for %s: %v\n", p, event, id, err)
	}
}

// do does something to a fake from the page. Runs on the plugin goroutine.
func (p *plugin) do(name string, action common.Action, count int) error {
	s, ok := p.byName[name]
	if !ok {
		return common.Refuse("there is no fake switch %q", name)
	}

	return s.do(action, count)
}

// getSwitches builds what the page shows. Runs on the plugin goroutine.
func (p *plugin) getSwitches() data.Switches {
	result := data.Switches{Items: make([]data.Switch, len(p.switches))}

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

		result.Items[i] = view
	}

	return result
}

// persist keeps the fakes as they are. Runs on the plugin goroutine.
func (p *plugin) persist() {
	persisted := config.PersistentConfigV1{Switches: make(map[string]config.SwitchStateV1, len(p.switches))}

	for _, s := range p.switches {
		persisted.Switches[s.name] = s.persisted()
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
