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
)

// SwitchStore is how internal/http reaches back into the plugin, which owns the fakes. It's implemented by the
// plugin's adapter, which crosses onto the plugin's own goroutine.
type SwitchStore interface {
	GetSwitches() (data.Switches, error)

	// Do does something to the named switch.
	Do(name string, action Action, count int) error
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
