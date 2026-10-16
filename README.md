# pmaas-plugin-fakes
Fake entities for testing and demo purposes: devices that exist only in PMAAS, for when the real ones are out of reach
(working from somewhere else) or there are none (a demo).

Each fake is registered as the entity a real device would be, with a state of its own, so everything that uses the real
thing works on it unchanged. The "Fakes" page shows what PMAAS has done to each, and changes it as the real thing could
be changed.

## A fake switch

```go
conf := fakes.NewPluginConfig()
porch := conf.AddSwitch("Porch light") // a device.ISwitch, usable from the moment the plugin has started
coreConfig.AddPlugin(fakes.NewPlugin(conf), config.PluginConfig{})

// ...and hand it to whatever would take a real one:
automations.TurnOn(porch)
```

It behaves as a device does, not as an idealised one:

- It **reports** the state it is in, which is not the state it was asked for. PMAAS sees changes as it would any
  device's, as events.
- The page has buttons to **press** the switch (the device changing state by itself, which PMAAS then observes), to take
  it **offline** (it refuses commands and keeps its last state), to make the **next N commands fail**, and to make it
  **accept commands and ignore them**: the ways a real device lets you down, for testing what happens next.
- It remembers the **commands** PMAAS has sent it, newest first, and whether each was accepted.
- Everything about it, including the settings above and the command history, is **kept across restarts** with the
  server's persistent configuration.

## A fake thermometer

```go
room := conf.AddThermometer("Room") // an environment.IWirelessThermometer, which is what the real ones are
```

It starts at 72°F and, like a real sensor, reports what it reads regularly, so the reading stays fresh. The page sets
its temperature (in °F, exactly or by a degree up or down), which PMAAS sees as a temperature change, and can make it
**stop reporting**, so that its last reading gets older: for testing what an automation does when it can't trust a
reading. The temperature and whether it is reporting are kept across restarts.

## A fake door sensor

```go
garage := conf.AddDoorSensor("Garage door") // a device.IDoorSensor, which is what the real ones are
```

It starts closed and online. The page opens and closes the door, which PMAAS sees as a position change, and takes the sensor
offline and brings it back: an offline sensor keeps the position it last reported, as a real one does, and can't report the
door moving. Its position is kept across restarts.

A switch or a door sensor can raise an alert when it goes offline, as a real device can (see `alert.WhenOffline` in
pmaas-spi). It is off unless asked for:

```go
conf.AddDoorSensor("Garage door").AlertWhenOffline(alert.WhenOffline(5*time.Minute, alert.Always(alert.SeverityWarning)))
```

The alert is raised once the device has been offline for that long, and cleared when it is back. With no severity it is a
warning, and a time of zero is as soon as it is noticed.

## A fake host

```go
router := conf.AddHost("Router")
netmonConfig.AddHost("router", "10.0.0.1").UseProber(router).AlertWhenUnreachable(2*time.Minute, nil)
```

A host that answers, until the page takes it down. It is for netmon (`pmaas-plugin-netmon`): give it as the host's prober
and netmon polls it as it would a real one, so everything it does when a host goes down, it does for this one: the
reachability events, the alert after a time, its escalation, and clearing it when the host is back. Whether it is down is kept
across restarts.

## Fakes for unit tests

The `fake` package has plain Go fakes that need no server, for testing code that takes a `device.ISwitch`:

```go
plug := fake.NewSwitch("Porch light", device.SwitchStateOff)
plug.FailWith(errors.New("offline")) // later commands fail
plug.Commands()                      // everything it was sent

room := fake.NewThermometer("Room", 22.5)     // degrees Celsius, as entities report them
garage := fake.NewDoor("Garage", device.DoorPositionClosed)
room.SetReading(30, time.Now().Add(-time.Hour)) // a reading that is an hour old
```
