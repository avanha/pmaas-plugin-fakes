package fakes

import (
	"fmt"
	"sync/atomic"

	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	spi "github.com/avanha/pmaas-spi"
	spicommon "github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/events"
)

// ButtonHandle is the assembly's handle on a fake button. It is a device.IButton, so it goes wherever a real one does (the
// beacon buttons of the bluetooth plugin are), it says its entity ID once the plugin has started, which is what an
// automation waiting for its presses needs (see trigger.ButtonPressed in pmaas-automations), and it works from the moment the
// plugin has started. Before then, and after the plugin stops, it answers as a button that has not been pressed.
type ButtonHandle struct {
	name     string
	stub     spicommon.StubHolder[device.IButton]
	entityId atomic.Pointer[string]
}

var _ device.IButton = (*ButtonHandle)(nil)

// Name is the button's name.
func (h *ButtonHandle) Name() string {
	return h.name
}

// EntityId is the ID the button is registered under once the plugin has started, which its events are from, or empty before
// then.
func (h *ButtonHandle) EntityId() string {
	if id := h.entityId.Load(); id != nil {
		return *id
	}

	return ""
}

func (h *ButtonHandle) GetButtonData() device.Button {
	stub, err := h.stub.Stub()
	if err != nil {
		return device.Button{Name: h.name}
	}

	return stub.GetButtonData()
}

// fakeButton is a button with nothing behind it. It is the entity PMAAS sees; the page presses it the ways a real one is
// pressed, and it announces each press as the real ones do (see device.ButtonPressedEvent). A button has no state worth
// keeping across a restart, so it is not kept. Owned by the plugin goroutine.
type fakeButton struct {
	plugin *plugin
	name   string
	id     string

	data device.Button

	stub *buttonStub
}

var _ device.IButton = (*fakeButton)(nil)

func (b *fakeButton) GetButtonData() device.Button {
	return b.data
}

// press is the button being pressed.
func (b *fakeButton) press(press device.ButtonPress) {
	b.data.Presses++
	b.data.LastPress, b.data.LastPressTime = press, b.plugin.now()

	b.plugin.broadcast(b.id, device.ButtonPressedEvent{EntityEvent: b.entityEvent(), Press: press, Time: b.data.LastPressTime, Count: b.data.Presses})
	b.plugin.broadcast(b.id, events.EntityStateChangedEvent{EntityEvent: b.entityEvent(), NewState: b.data})
}

// do does something to the button from the page.
func (b *fakeButton) do(action common.Action) error {
	switch action {
	case common.ActionPressSingle:
		b.press(device.PressSingle)
	case common.ActionPressDouble:
		b.press(device.PressDouble)
	case common.ActionPressTriple:
		b.press(device.PressTriple)
	case common.ActionPressLong:
		b.press(device.PressLong)
	default:
		return common.Refuse("a button has no action %q", action)
	}

	return nil
}

func (b *fakeButton) entityEvent() events.EntityEvent {
	return events.EntityEvent{Id: b.id, EntityType: device.IButtonType, Name: b.name}
}

func (b *fakeButton) getStub(container spi.IPMAASContainer) *buttonStub {
	if b.stub == nil {
		b.stub = &buttonStub{}
		b.stub.wrapper.Store(&spicommon.ThreadSafeEntityWrapper[device.IButton]{Container: container, Entity: b})
	}

	return b.stub
}

func (b *fakeButton) closeStub() {
	if b.stub != nil {
		b.stub.wrapper.Store(nil)
		b.stub = nil
	}
}

// buttonStub is the button as every other goroutine sees it: each call hops onto the plugin's.
type buttonStub struct {
	wrapper atomic.Pointer[spicommon.ThreadSafeEntityWrapper[device.IButton]]
}

var _ device.IButton = (*buttonStub)(nil)

func (s *buttonStub) GetButtonData() device.Button {
	return spicommon.ThreadSafeEntityWrapperExecValueFunc(
		s.wrapper.Load(),
		func(target device.IButton) device.Button { return target.GetButtonData() })
}

// startButton makes a fake button, which has never been pressed, and registers it.
func (p *plugin) startButton(handle *ButtonHandle) {
	b := &fakeButton{plugin: p, name: handle.name, data: device.Button{Name: handle.name}}

	id, err := p.container.RegisterEntity(
		handle.name, device.IButtonType, handle.name,
		func() (any, error) { return b.getStub(p.container), nil })
	if err != nil {
		fmt.Printf("%T unable to register the fake button %q: %v\n", p, handle.name, err)
		return
	}

	b.id = id
	handle.entityId.Store(&id)
	p.buttons = append(p.buttons, b)
	p.buttonByName[handle.name] = b

	event := spicommon.StubAvailableEvent[device.IButton]{EntityEvent: b.entityEvent(), EntityStub: b.getStub(p.container)}
	if err := handle.stub.Provide(p.container, event); err != nil {
		fmt.Printf("%T %v\n", p, err)
	}
}
