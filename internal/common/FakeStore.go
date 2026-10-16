package common

import (
	"fmt"

	"github.com/avanha/pmaas-plugin-fakes/data"
)

// Action is something done to a fake switch from the page, the way someone with the real device in front of them
// could.
type Action string

const (
	// ActionReportOn and ActionReportOff are the device changing state by itself, as when someone presses its
	// button: it reports the new state, and PMAAS sees the change.
	ActionReportOn  Action = "report-on"
	ActionReportOff Action = "report-off"

	// ActionOffline and ActionOnline take the device off the network and put it back.
	ActionOffline Action = "offline"
	ActionOnline  Action = "online"

	// ActionFailNext makes the next Count commands fail, as a device that is flaky would.
	ActionFailNext Action = "fail-next"

	// ActionIgnoreCommands makes the device accept commands and not act on them, and ActionFollowCommands has it
	// act on them, as it normally does.
	ActionIgnoreCommands Action = "ignore-commands"
	ActionFollowCommands Action = "follow-commands"

	// ActionClearHistory forgets the commands it has been sent.
	ActionClearHistory Action = "clear-history"

	// ActionSetTemperature makes a thermometer read the value, in degrees Fahrenheit, and ActionAdjustTemperature
	// changes what it reads by the value, which can be negative.
	ActionSetTemperature    Action = "set-temperature"
	ActionAdjustTemperature Action = "adjust-temperature"

	// ActionDoorOpen and ActionDoorClose are a door sensor's door moving, which it reports. ActionOffline and
	// ActionOnline apply to a door sensor too: the sensor going off the network and coming back, keeping the last
	// position it reported.
	ActionDoorOpen  Action = "door-open"
	ActionDoorClose Action = "door-close"

	// ActionHostDown and ActionHostUp make a host unreachable and reachable, as far as netmon polling it can tell.
	ActionHostDown Action = "host-down"
	ActionHostUp   Action = "host-up"

	// ActionStopReporting makes a thermometer stop reporting, as one whose battery has died does: what it last read
	// stays, and gets older. ActionResumeReporting has it report again.
	ActionStopReporting   Action = "stop-reporting"
	ActionResumeReporting Action = "resume-reporting"
)

// FakeStore is how internal/http reaches back into the plugin, which owns the fakes. It's implemented by the
// plugin's adapter, which crosses onto the plugin's own goroutine.
type FakeStore interface {
	GetFakes() (data.Fakes, error)

	// Do does something to the named fake. Count is for the actions that take a whole number, and Value for those that
	// take a number of degrees.
	Do(name string, action Action, count int, value float64) error
}

// RequestError is that what was asked for can't be done, for a reason the person who asked can do something about:
// there is no such switch, or the device is offline. Its message is fit to show them. Anything else that goes wrong
// is not.
type RequestError struct {
	Message string
}

func (e *RequestError) Error() string {
	return e.Message
}

// Refuse makes a RequestError.
func Refuse(format string, args ...any) error {
	return &RequestError{Message: fmt.Sprintf(format, args...)}
}
