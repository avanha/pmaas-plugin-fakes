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

## Fakes for unit tests

The `fake` package has plain Go fakes that need no server, for testing code that takes a `device.ISwitch`:

```go
plug := fake.NewSwitch("Porch light", device.SwitchStateOff)
plug.FailWith(errors.New("offline")) // later commands fail
plug.Commands()                      // everything it was sent
```
