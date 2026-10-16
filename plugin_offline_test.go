package fakes

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi/alert"
	"github.com/avanha/pmaas-spi/clock"
	"github.com/avanha/pmaas-spi/events"
)

// minute is the clock saying a minute has begun, to the plugin's receiver, on the plugin's goroutine.
func (f *fixture) minute() {
	f.t.Helper()

	f.container.mu.Lock()
	receiver := f.container.receiver
	f.container.mu.Unlock()

	if receiver == nil {
		f.t.Fatal("the plugin isn't listening for the clock")
	}

	f.container.onPluginGoroutine(func() {
		_ = receiver(&events.EventInfo{Event: clock.MinuteChangedEvent{Time: f.plugin.now()}})
	})
}

// minutes lets a span of virtual time go by, a minute at a time with the clock saying so.
func minutes(f *fixture, span time.Duration) {
	for ; span > 0; span -= time.Minute {
		time.Sleep(min(span, time.Minute))
		f.minute()
	}
}

func alertEvents(broadcasts []any) (raised []alert.RaisedEvent, cleared []alert.ClearedEvent) {
	for _, b := range broadcasts {
		switch e := b.(type) {
		case alert.RaisedEvent:
			raised = append(raised, e)
		case alert.ClearedEvent:
			cleared = append(cleared, e)
		}
	}

	return raised, cleared
}

func TestOfflineAlert_IsOffUnlessAskedFor(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	_ = f.doTo("Garage door", common.ActionOffline, 0)
	_ = f.doTo("Porch", common.ActionOffline, 0)
	f.advance(24 * time.Hour)
	f.minute()

	if raised, cleared := alertEvents(f.container.broadcastsOf()); len(raised)+len(cleared) != 0 {
		t.Errorf("alerts with none asked for: %v %v", raised, cleared)
	}
}

func TestOfflineAlert_ASensorWithNoTimeAlertsAtOnceAndClearsWhenBack(t *testing.T) {
	f := newFixture(t, nil)
	f.door.AlertWhenOffline(alert.WhenOffline(0, alert.Always(alert.SeverityCritical)))
	f.start()

	_ = f.doTo("Garage door", common.ActionOffline, 0)

	raised, _ := alertEvents(f.container.broadcastsOf())
	if len(raised) != 1 || raised[0].Source != "fakes" || raised[0].Key != "offline:Garage door" ||
		raised[0].Severity != alert.SeverityCritical || raised[0].Title != "Offline: Garage door" {
		t.Fatalf("raised %+v", raised)
	}

	_ = f.doTo("Garage door", common.ActionOnline, 0)

	if _, cleared := alertEvents(f.container.broadcastsOf()); len(cleared) != 1 || cleared[0].Key != "offline:Garage door" {
		t.Fatalf("cleared %+v", cleared)
	}
}

func TestOfflineAlert_ASwitchAlertsToo_ByItsOwnKey(t *testing.T) {
	f := newFixture(t, nil)
	f.handle.AlertWhenOffline(alert.WhenOffline(0, nil))
	f.start()

	_ = f.doTo("Porch", common.ActionOffline, 0)

	raised, _ := alertEvents(f.container.broadcastsOf())
	if len(raised) != 1 || raised[0].Key != "offline:Porch" || raised[0].Severity != alert.SeverityWarning {
		t.Fatalf("raised %+v", raised)
	}
}

func TestOfflineAlert_WaitsForItsTimeOnTheVirtualClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, nil)
		f.plugin.now = time.Now
		f.door.AlertWhenOffline(alert.WhenOffline(5*time.Minute, nil))
		f.start()

		_ = f.doTo("Garage door", common.ActionOffline, 0)

		minutes(f, 4*time.Minute+30*time.Second)

		if raised, _ := alertEvents(f.container.broadcastsOf()); len(raised) != 0 {
			t.Fatalf("raised too early: %+v", raised)
		}

		// Nothing has changed, but the time has come: the plugin looks at each minute.
		minutes(f, 90*time.Second)

		if raised, _ := alertEvents(f.container.broadcastsOf()); len(raised) != 1 {
			t.Fatalf("raised %+v", raised)
		}

		minutes(f, 10*time.Minute)

		if raised, _ := alertEvents(f.container.broadcastsOf()); len(raised) != 1 {
			t.Fatalf("raised again: %+v", raised)
		}

		f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })
	})
}

func TestOfflineAlert_ListensForTheClockWhileRunningAndNotAfter(t *testing.T) {
	f := newFixture(t, nil)
	f.start()

	if f.container.receiver == nil {
		t.Fatal("not listening for the clock")
	}

	f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })

	if f.container.receiver != nil || len(f.container.receiversDeregistered) != 1 {
		t.Errorf("still listening: %v", f.container.receiversDeregistered)
	}
}
