package fakes

import (
	"testing"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi/device"
)

type buttonFixture struct {
	*fixture
	button *ButtonHandle
}

func newButtonFixture(t *testing.T) *buttonFixture {
	t.Helper()

	f := &fixture{t: t, container: newFakeContainer(t), clock: epoch}
	conf := NewPluginConfig()
	b := conf.AddButton("Couch")
	f.plugin = NewPlugin(conf).(*plugin)
	f.plugin.now = func() time.Time { return f.clock }

	return &buttonFixture{fixture: f, button: b}
}

func (f *buttonFixture) press(action common.Action) error {
	f.t.Helper()

	var err error

	f.container.onPluginGoroutine(func() { err = f.plugin.do("Couch", action, 0, 0) })

	return err
}

func pressEvents(broadcasts []any) []device.ButtonPressedEvent {
	var result []device.ButtonPressedEvent

	for _, b := range broadcasts {
		if e, ok := b.(device.ButtonPressedEvent); ok {
			result = append(result, e)
		}
	}

	return result
}

func TestButton_IsUnpressedBeforeStartAndHasNoEntityId(t *testing.T) {
	f := newButtonFixture(t)

	if got := f.button.GetButtonData(); got.Name != "Couch" || got.Presses != 0 {
		t.Errorf("before: %+v", got)
	}

	if f.button.EntityId() != "" {
		t.Errorf("entity ID before start: %q", f.button.EntityId())
	}

	f.start()

	if f.button.EntityId() != "fakes_Couch" {
		t.Errorf("entity ID: %q", f.button.EntityId())
	}
}

func TestButton_EachKindOfPressIsAnnouncedWithItsCount(t *testing.T) {
	f := newButtonFixture(t)
	f.start()

	want := map[common.Action]device.ButtonPress{
		common.ActionPressSingle: device.PressSingle,
		common.ActionPressDouble: device.PressDouble,
		common.ActionPressTriple: device.PressTriple,
		common.ActionPressLong:   device.PressLong,
	}

	for _, action := range []common.Action{
		common.ActionPressSingle, common.ActionPressDouble, common.ActionPressTriple, common.ActionPressLong,
	} {
		f.advance(time.Second)

		if err := f.press(action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}

	got := pressEvents(f.container.broadcastsOf())
	if len(got) != 4 {
		t.Fatalf("got %d press events: %+v", len(got), got)
	}

	for i, action := range []common.Action{
		common.ActionPressSingle, common.ActionPressDouble, common.ActionPressTriple, common.ActionPressLong,
	} {
		e := got[i]
		if e.Press != want[action] || e.Count != uint64(i+1) || e.Id != "fakes_Couch" || !e.Time.Equal(epoch.Add(time.Duration(i+1)*time.Second)) {
			t.Errorf("event %d: %+v", i, e)
		}
	}

	data := f.button.GetButtonData()
	if data.Presses != 4 || data.LastPress != device.PressLong {
		t.Errorf("state: %+v", data)
	}
}

func TestButton_AnActionItDoesNotHaveIsRefusedAndAnnouncesNothing(t *testing.T) {
	f := newButtonFixture(t)
	f.start()

	if err := f.press(common.ActionHostDown); err == nil {
		t.Error("a button was told to go down")
	}

	if len(pressEvents(f.container.broadcastsOf())) != 0 || f.button.GetButtonData().Presses != 0 {
		t.Error("a refused action counted as a press")
	}
}

func TestButton_IsGoneAfterStop(t *testing.T) {
	f := newButtonFixture(t)
	f.start()
	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	if f.button.EntityId() != "" {
		t.Errorf("entity ID after stop: %q", f.button.EntityId())
	}

	if got := f.button.GetButtonData(); got.Presses != 0 || got.Name != "Couch" {
		t.Errorf("after: %+v", got)
	}

	if len(f.container.deregistered) != 1 || f.container.deregistered[0] != "fakes_Couch" {
		t.Errorf("deregistered %v", f.container.deregistered)
	}

}

func TestPage_ListsTheButton(t *testing.T) {
	f := newButtonFixture(t)
	f.start()
	f.press(common.ActionPressDouble)

	var got []string

	f.container.onPluginGoroutine(func() {
		for _, b := range f.plugin.getFakes().Buttons {
			got = append(got, b.Name+"/"+b.LastPress)
		}
	})

	if len(got) != 1 || got[0] != "Couch/double" {
		t.Errorf("got %v", got)
	}
}
