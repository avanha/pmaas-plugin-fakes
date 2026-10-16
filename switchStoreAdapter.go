package fakes

import (
	"github.com/avanha/pmaas-plugin-fakes/data"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	spi "github.com/avanha/pmaas-spi"
)

// Force implementation of common.SwitchStore
var _ common.SwitchStore = (*switchStoreAdapter)(nil)

// switchStoreAdapter is how HTTP requests reach the fakes. They come in on arbitrary goroutines, and the fakes belong
// to the plugin's own, so each call runs there and waits for the answer.
type switchStoreAdapter struct {
	parent *plugin
}

func (a *switchStoreAdapter) GetSwitches() (data.Switches, error) {
	return spi.Exec(a.parent.container, a.parent.getSwitches)
}

func (a *switchStoreAdapter) Do(name string, action common.Action, count int) error {
	result, err := spi.Exec(a.parent.container, func() error { return a.parent.do(name, action, count) })
	if err != nil {
		return err
	}

	return result
}
