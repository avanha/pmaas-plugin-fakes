package config

import "time"

// PersistentConfigV1 is the plugin's state persisted across restarts via
// IPMAASContainer.SaveConfig/LoadConfig: what each fake is, as it was left, by its name. A fake that was
// turned on by hand is still on after a restart, as a real one would be.
type PersistentConfigV1 struct {
	Switches     map[string]SwitchStateV1
	Thermometers map[string]ThermometerStateV1
	DoorSensors  map[string]DoorSensorStateV1
	Hosts        map[string]HostStateV1
}

// DoorSensorStateV1 is one fake door sensor as it was left.
type DoorSensorStateV1 struct {
	// Position is "open", "closed" or "unknown", kept as words, and HasData whether it had ever reported.
	Position       string
	HasData        bool
	LastChangeTime time.Time
	LastUpdateTime time.Time

	// Offline is that the sensor was off the network, and OfflineSince or OnlineSince when that began.
	Offline      bool
	OfflineSince time.Time
	OnlineSince  time.Time
}

// HostStateV1 is one fake host as it was left.
type HostStateV1 struct {
	Down  bool
	Since time.Time
}

// ThermometerStateV1 is one fake thermometer as it was left.
type ThermometerStateV1 struct {
	HasData bool

	// Celsius is what it read, in the unit entities report in.
	Celsius float32

	// StopReporting is that it was not reporting, so that what it read gets older, and LastUpdateTime is when it last
	// did. A thermometer that is reporting has its time brought up to date when it starts, as a real one would on
	// reconnecting.
	StopReporting  bool
	LastUpdateTime time.Time
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
