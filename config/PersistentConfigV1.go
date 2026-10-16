package config

import "time"

// PersistentConfigV1 is the plugin's state persisted across restarts via
// IPMAASContainer.SaveConfig/LoadConfig: what each fake is, as it was left, by its name. A fake that was
// turned on by hand is still on after a restart, as a real one would be.
type PersistentConfigV1 struct {
	Switches map[string]SwitchStateV1
}

// SwitchStateV1 is one fake switch as it was left.
type SwitchStateV1 struct {
	// State is "on", "off" or "unknown", and HasData whether it had ever reported. They are kept as words so that
	// what is saved does not change meaning if the plugin's own numbering does.
	State          string
	HasData        bool
	LastChangeTime time.Time
	LastUpdateTime time.Time

	// Offline is that the device was off the network, and OfflineSince or OnlineSince when that began.
	Offline      bool
	OfflineSince time.Time
	OnlineSince  time.Time

	// IgnoreCommands is that the device did not act on what it was asked. It is the opposite of what a device
	// normally does, so that the zero value, and a file written before it existed, is a device that does.
	IgnoreCommands bool

	// FailNext is how many more commands are to fail.
	FailNext int

	// Commanded is what PMAAS last asked for, in the same words as State, and CommandedAt when.
	Commanded   string
	CommandedAt time.Time

	// History is the latest commands, newest first.
	History []CommandV1
}

// CommandV1 is one command PMAAS sent to a fake.
type CommandV1 struct {
	At       time.Time
	State    string
	Accepted bool

	// Result says why a command that was not accepted was not.
	Result string
}
