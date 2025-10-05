//go:build !js && !test

package audio

// PlayBatch dispatches multiple sound requests. On desktop, this simply
// iterates and calls PlayParamsAt for each item.
func PlayBatch(reqs []BatchParam) {
	for i := range reqs {
		r := reqs[i]
		when := 0.0
		if r.HasWhen {
			when = r.When
		}
		PlayParamsAt(r.ID, r.Vol, r.Pitch, r.Dur, when)
	}
}
