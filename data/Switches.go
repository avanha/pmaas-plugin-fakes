// Package data is what the fakes page shows.
package data

import "time"

// Switches is everything the page shows.
type Switches struct {
	Items []Switch
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
