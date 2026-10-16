package http

import (
	"bytes"
	"errors"
	"net/http"
	"path"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/avanha/pmaas-plugin-fakes/data"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
)

// render renders the template the way PMAAS does. Its template engine plugin uses text/template, which doesn't escape
// anything, so a template has to do that itself: rendering with html/template here would escape automatically and
// hide a template that doesn't.
func render(t *testing.T, fakes data.Fakes) string {
	t.Helper()

	name := path.Base(devicesTemplate.Paths[0])
	tmpl, err := template.New(name).Funcs(devicesTemplate.FuncMap).ParseFS(contentFS, "content/"+devicesTemplate.Paths[0])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, name, &fakes); err != nil {
		t.Fatalf("execute: %v", err)
	}

	return out.String()
}

func sw(name string) data.Switch {
	return data.Switch{Name: name, State: "On", StateClass: "on", LastChangeTime: time.Now().Add(-2 * time.Hour)}
}

func TestTemplate_Empty(t *testing.T) {
	out := render(t, data.Fakes{})

	if !strings.Contains(out, "No fake devices are configured") || strings.Contains(out, "fake-button") {
		t.Errorf("empty page:\n%s", out)
	}
}

func TestTemplate_ASwitchShowsWhatItReportsWhatWasAskedAndTheControls(t *testing.T) {
	porch := sw("Porch")
	porch.Commanded, porch.CommandedAt = "On", time.Now().Add(-5*time.Minute)
	porch.History = []data.Command{
		{At: time.Date(2026, 10, 16, 15, 4, 5, 0, time.Local), State: "On", Accepted: true},
		{At: time.Date(2026, 10, 16, 15, 3, 0, 0, time.Local), State: "Off", Result: "the device is offline"},
	}

	// HTML doesn't care where the lines break, so neither does the test.
	out := strings.Join(strings.Fields(render(t, data.Fakes{Switches: []data.Switch{porch}})), " ")

	for _, want := range []string{
		`data-name="Porch"`, `state-badge on">On`, "changed 2h 0m ago", "On 5m ago",
		`data-action="report-on"`, `data-action="report-off"`, `data-action="offline"`, `data-action="ignore-commands"`,
		`data-action="fail-next"`, `data-action="clear-history"`,
		"3:04:05 PM asked for On: accepted", "3:03:00 PM asked for Off: refused, the device is offline",
		`command rejected`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	for _, unwanted := range []string{"Offline</span>", `data-action="online"`, `data-action="follow-commands"`, "disabled"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("unexpectedly contains %q in:\n%s", unwanted, out)
		}
	}
}

func TestTemplate_NothingAskedYet(t *testing.T) {
	out := render(t, data.Fakes{Switches: []data.Switch{sw("Porch")}})

	for _, want := range []string{"nothing yet", ">None<"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	if strings.Contains(out, "clear-history") {
		t.Error("a clear button with nothing to clear")
	}
}

func TestTemplate_AnOfflineSwitchCanOnlyBeBroughtBack(t *testing.T) {
	offline := sw("Porch")
	offline.Offline = true

	out := render(t, data.Fakes{Switches: []data.Switch{offline}})

	for _, want := range []string{`class="fake-device fake-switch offline"`, "Offline</span>", `data-action="online"`, `data-action="report-on" disabled`, `data-action="report-off" disabled`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	if strings.Contains(out, `data-action="offline"`) {
		t.Errorf("offers to take an offline device offline:\n%s", out)
	}
}

func TestTemplate_ASwitchThatIgnoresCommandsSaysSoAndOffersToStop(t *testing.T) {
	ignoring := sw("Porch")
	ignoring.IgnoreCommands, ignoring.FailNext = true, 4

	out := render(t, data.Fakes{Switches: []data.Switch{ignoring}})

	for _, want := range []string{"accepts commands but does not act on them", `data-action="follow-commands"`, `value="4"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	if strings.Contains(out, `data-action="ignore-commands"`) {
		t.Error("offers to ignore commands that are being ignored")
	}
}

// A name is chosen by the assembly, but a result can carry text from elsewhere. All of it must reach the page as text.
func TestTemplate_EscapesEverythingThatIsText(t *testing.T) {
	hostile := sw(`<b>name</b>" onclick="alert(1)"`)
	hostile.State = `<blink>state</blink>`
	hostile.StateClass = `x" onmouseover="alert(2)`
	hostile.Commanded, hostile.CommandedAt = `<i>asked</i>`, time.Now()
	hostile.History = []data.Command{{At: time.Now(), State: `<u>state</u>`, Result: `<script>alert("result")</script>`}}

	out := render(t, data.Fakes{Switches: []data.Switch{hostile}})

	for _, dangerous := range []string{
		"<blink>", "<i>asked", "<u>state", "<script>", "<b>name", `onclick="alert(1)"`, `onmouseover="alert(2)"`,
	} {
		if strings.Contains(out, dangerous) {
			t.Errorf("unescaped %q in:\n%s", dangerous, out)
		}
	}
}

type fakeStore struct {
	did    []string
	counts []int
	values []float64
	err    error
}

func (s *fakeStore) GetFakes() (data.Fakes, error) { return data.Fakes{}, s.err }

func (s *fakeStore) Do(name string, action common.Action, count int, value float64) error {
	s.did = append(s.did, name+":"+string(action))
	s.counts = append(s.counts, count)
	s.values = append(s.values, value)

	return s.err
}

func TestControlRequest(t *testing.T) {
	post := &http.Request{Method: http.MethodPost}

	t.Run("does it", func(t *testing.T) {
		store := &fakeStore{}
		h := &Handler{store: store}

		response, err := h.handleHttpControlRequest(nil, post, &controlRequest{Name: "Porch", Action: "set-temperature", Count: 3, Value: 71.5})
		if err != nil || !response.(controlResponse).OK {
			t.Fatalf("response %+v, err %v", response, err)
		}

		if len(store.did) != 1 || store.did[0] != "Porch:set-temperature" || store.counts[0] != 3 || store.values[0] != 71.5 {
			t.Errorf("did %v %v", store.did, store.counts)
		}
	})

	t.Run("only POST", func(t *testing.T) {
		store := &fakeStore{}
		h := &Handler{store: store}

		if _, err := h.handleHttpControlRequest(nil, &http.Request{Method: http.MethodGet}, &controlRequest{Name: "a", Action: "b"}); err == nil {
			t.Error("a GET was accepted")
		}

		if len(store.did) != 0 {
			t.Error("a rejected request still did something")
		}
	})

	t.Run("needs a name and an action", func(t *testing.T) {
		for _, body := range []any{&controlRequest{}, &controlRequest{Name: "a"}, &controlRequest{Action: "b"}, nil, "x"} {
			store := &fakeStore{}
			h := &Handler{store: store}

			if _, err := h.handleHttpControlRequest(nil, post, body); err == nil {
				t.Errorf("%v was accepted", body)
			}

			if len(store.did) != 0 {
				t.Errorf("%v still did something", body)
			}
		}
	})

	t.Run("says what was wrong with the request", func(t *testing.T) {
		h := &Handler{store: &fakeStore{err: common.Refuse("the device is offline")}}

		_, err := h.handleHttpControlRequest(nil, post, &controlRequest{Name: "a", Action: "b"})
		if err == nil || err.Error() != "the device is offline" {
			t.Errorf("error %v", err)
		}
	})

	t.Run("keeps anything else to itself", func(t *testing.T) {
		h := &Handler{store: &fakeStore{err: errors.New("secret internal detail: /var/pmaas/db")}}

		_, err := h.handleHttpControlRequest(nil, post, &controlRequest{Name: "a", Action: "b"})
		if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "/var") {
			t.Errorf("error %v", err)
		}
	})
}

func TestRelativeTime(t *testing.T) {
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, "never"},
		{time.Now().Add(-10 * time.Second), "just now"},
		{time.Now().Add(-5*time.Minute - 20*time.Second), "5m ago"},
		{time.Now().Add(-2*time.Hour - 5*time.Minute), "2h 5m ago"},
		{time.Now().Add(-3*24*time.Hour - 4*time.Hour - time.Minute), "3d 4h ago"},
	} {
		if got := relativeTime(c.t); got != c.want {
			t.Errorf("%v: got %q, want %q", c.t, got, c.want)
		}
	}
}

func thermometer(name string) data.Thermometer {
	return data.Thermometer{
		Name: name, HasData: true, Fahrenheit: 78.1, Celsius: 25.6, Reported: time.Now().Add(-3 * time.Minute),
	}
}

func TestTemplate_AThermometerShowsItsReadingAndControls(t *testing.T) {
	out := strings.Join(strings.Fields(render(t, data.Fakes{Thermometers: []data.Thermometer{thermometer("Room")}})), " ")

	for _, want := range []string{
		`fake-device fake-thermometer" data-name="Room"`, `<span class="kind">thermometer</span>`, "78.1°F (25.6°C)", "3m ago",
		`class="temperature" type="number" step="0.1" min="-100" max="250" value="78.1" >`,
		`data-action="set-temperature" >`, `data-action="adjust-temperature" data-delta="-1" >`,
		`data-action="adjust-temperature" data-delta="1" >`, `data-action="stop-reporting"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	for _, unwanted := range []string{"Not reporting", "resume-reporting", "disabled", "No fake devices"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("unexpectedly contains %q in:\n%s", unwanted, out)
		}
	}
}

func TestTemplate_AThermometerThatStoppedReportingCanOnlyBeResumed(t *testing.T) {
	stopped := thermometer("Room")
	stopped.StopReporting = true

	out := strings.Join(strings.Fields(render(t, data.Fakes{Thermometers: []data.Thermometer{stopped}})), " ")

	for _, want := range []string{
		"fake-thermometer stale", "Not reporting", `data-action="resume-reporting"`,
		`data-action="set-temperature" disabled`, `data-action="adjust-temperature" data-delta="-1" disabled`,
		`data-action="adjust-temperature" data-delta="1" disabled`, `value="78.1" disabled`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	if strings.Contains(out, `data-action="stop-reporting"`) {
		t.Error("offers to stop a thermometer that has stopped")
	}
}

func TestTemplate_AThermometerWithNoReading(t *testing.T) {
	out := render(t, data.Fakes{Thermometers: []data.Thermometer{{Name: "Room"}}})

	if !strings.Contains(out, "no reading") || strings.Contains(out, "°F (") {
		t.Errorf("page:\n%s", out)
	}
}

func TestTemplate_SwitchesAndThermometersTogetherAndOnlyEmptyWhenBothAre(t *testing.T) {
	both := render(t, data.Fakes{Switches: []data.Switch{sw("Porch")}, Thermometers: []data.Thermometer{thermometer("Room")}})

	if !strings.Contains(both, "fake-switch") || !strings.Contains(both, "fake-thermometer") || strings.Contains(both, "No fake devices") {
		t.Errorf("both:\n%s", both)
	}

	only := render(t, data.Fakes{Thermometers: []data.Thermometer{thermometer("Room")}})

	if strings.Contains(only, "No fake devices") {
		t.Errorf("a page with a thermometer says there are none:\n%s", only)
	}
}

func TestTemplate_EscapesAThermometersName(t *testing.T) {
	out := render(t, data.Fakes{Thermometers: []data.Thermometer{thermometer(`<script>alert("name")</script>" onclick="alert(1)"`)}})

	for _, dangerous := range []string{"<script>", `onclick="alert(1)"`} {
		if strings.Contains(out, dangerous) {
			t.Errorf("unescaped %q in:\n%s", dangerous, out)
		}
	}
}
