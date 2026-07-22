//go:build js

package userprefs

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

func (b *lsInstrumentBackend) write(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// localStorage.setItem can throw (quota / privacy mode); degrade.
			err = nil
		}
	}()
	ls := localStorageOrNil()
	if ls.IsUndefined() {
		return nil
	}
	ls.Call("setItem", b.key, string(data))
	return nil
}
