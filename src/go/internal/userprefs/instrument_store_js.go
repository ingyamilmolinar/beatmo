//go:build js

package userprefs

import (
	"errors"
	"fmt"
)

// lsInstrumentBackend persists the instrument store to its own localStorage key,
// sibling to the prefs key. Degrades to in-memory (nil reads) when localStorage
// is unreachable, mirroring the prefs store.
type lsInstrumentBackend struct {
	key string
}

func newInstrumentBackend(_ Options) instrumentBackend {
	return &lsInstrumentBackend{key: InstrumentsLocalStorageKey}
}

func (b *lsInstrumentBackend) read() ([]byte, error) {
	ls := localStorageOrNil()
	if ls.IsUndefined() {
		return nil, nil
	}
	val := ls.Call("getItem", b.key)
	if val.IsNull() || val.IsUndefined() {
		return nil, nil
	}
	return []byte(val.String()), nil
}

// write reports a write the browser refused (quota full, private mode) or
// cannot make (no localStorage): the user library's Save As must tell the user
// rather than claim a save that is gone on reload.
func (b *lsInstrumentBackend) write(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("localStorage refused the write: %v", r)
		}
	}()
	ls := localStorageOrNil()
	if ls.IsUndefined() {
		return errors.New("localStorage is unavailable")
	}
	ls.Call("setItem", b.key, string(data))
	return nil
}
