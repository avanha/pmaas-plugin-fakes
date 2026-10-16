package fakes

import (
	"github.com/avanha/pmaas-plugin-fakes/data"
	"github.com/avanha/pmaas-plugin-fakes/internal/common"
	spi "github.com/avanha/pmaas-spi"
)

// Force implementation of common.FakeStore
var _ common.FakeStore = (*fakeStoreAdapter)(nil)

// fakeStoreAdapter is how HTTP requests reach the fakes. They come in on arbitrary goroutines, and the fakes belong
// to the plugin's own, so each call runs there and waits for the answer.
type fakeStoreAdapter struct {
	parent *plugin
}

func (a *fakeStoreAdapter) GetFakes() (data.Fakes, error) {
	return spi.Exec(a.parent.container, a.parent.getFakes)
}

func (a *fakeStoreAdapter) Do(name string, action common.Action, count int, value float64) error {
	result, err := spi.Exec(a.parent.container, func() error { return a.parent.do(name, action, count, value) })
	if err != nil {
		return err
	}

	return result
}
