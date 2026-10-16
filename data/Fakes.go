// Package data is what the fakes page shows.
package data

import "time"

// Fakes is everything the page shows.
type Fakes struct {
	Switches     []Switch
	Thermometers []Thermometer
}

// Thermometer is one fake thermometer as it stands.
type Thermometer struct {
	Name string

	// HasData is that it has ever reported. Fahrenheit and Celsius are what it reads, and Reported is when it last
	// reported, which a thermometer that has stopped reporting leaves behind.
	HasData    bool
	Fahrenheit float32
	Celsius    float32
	Reported   time.Time

	StopReporting bool
}

// Switch is one fake switch as it stands.
type Switch struct {
	Name string

	// State is what the fake last reported, in words: "On", "Off" or "Unknown", and StateClass the same as a word safe
	// to use as a style.
	State      string
	StateClass string

	Offline        bool
	LastChangeTime time.Time
	LastUpdateTime time.Time

	// Commanded is what PMAAS last asked for, in words, and empty if it never has.
	Commanded   string
	CommandedAt time.Time

	IgnoreCommands bool
	FailNext       int

	History []Command
}

// Command is one thing PMAAS asked of a fake.
type Command struct {
	At       time.Time
	State    string
	Accepted bool
	Result   string
}
