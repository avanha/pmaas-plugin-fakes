package fakes

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	"github.com/avanha/pmaas-spi/alert"
)

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
	f.container.onPluginGoroutine(f.plugin.report)

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

		time.Sleep(4*time.Minute + 30*time.Second)

		if raised, _ := alertEvents(f.container.broadcastsOf()); len(raised) != 0 {
			t.Fatalf("raised too early: %+v", raised)
		}

		// Nothing has changed, but the time has come: the plugin looks every minute.
		time.Sleep(90 * time.Second)

		if raised, _ := alertEvents(f.container.broadcastsOf()); len(raised) != 1 {
			t.Fatalf("raised %+v", raised)
		}

		time.Sleep(10 * time.Minute)

		if raised, _ := alertEvents(f.container.broadcastsOf()); len(raised) != 1 {
			t.Fatalf("raised again: %+v", raised)
		}

		f.container.onPluginGoroutine(func() { <-f.plugin.Stop() })
	})
}
