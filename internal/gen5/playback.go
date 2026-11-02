package gen5

import "fmt"

func rangeDuration(a *Animation, first, last, step int) int {
	total := 0
	for i := first; i <= last; i += step {
		if i >= 0 && i < len(a.Frames) {
			total += a.Frames[i].Duration
		}
	}
	return total
}

func selectRangeIndex(a *Animation, first, last, step, tick int) int {
	chosen := first
	for i := first; i <= last; i += step {
		chosen = i
		d := a.Frames[i].Duration
		if d > 0 {
			if tick < d {
				return i
			}
			tick -= d
		}
	}
	return chosen
}

// FrameIndexAt resolves which frame of the animation plays at tick.
func FrameIndexAt(bank *Bank, index, tick int) (int, error) {
	if bank == nil || index < 0 || index >= len(bank.Animations) {
		return 0, fmt.Errorf("missing animation %d", index)
	}
	a := &bank.Animations[index]
	if len(a.Frames) == 0 {
		return 0, fmt.Errorf("animation %d has no frames", index)
	}
	if tick < 0 {
		tick = 0
	}

	n := len(a.Frames)
	loopStart := a.LoopStart + 1
	duration := rangeDuration(a, 0, n-1, 1)
	if duration <= 0 {
		return 0, nil
	}

	loop := a.Playback == 2 || a.Playback == 4
	ping := a.Playback == 3 || a.Playback == 4
	reverseStart := n - 1
	reverseEnd := 1
	if loop {
		reverseEnd = loopStart
	}
	reverseDuration := 0
	if ping && reverseStart >= reverseEnd {
		reverseDuration = rangeDuration(a, reverseStart-1, reverseEnd-1, -1)
	}
	sequence := duration + reverseDuration

	if loop {
		prefix := rangeDuration(a, 0, loopStart-2, 1)
		period := rangeDuration(a, loopStart-1, n-1, 1) + reverseDuration
		if tick >= sequence {
			if period > 0 {
				tick = prefix + (tick-prefix)%period
			} else {
				tick %= sequence
			}
		}
	} else if tick > sequence-1 {
		tick = sequence - 1
	}

	if tick < duration || !ping || reverseDuration == 0 {
		return selectRangeIndex(a, 0, n-1, 1, tick), nil
	}
	return selectRangeIndex(a, reverseStart-1, reverseEnd-1, -1, tick-duration), nil
}

// FrameAt resolves the transform bank's animation index plays at tick.
func FrameAt(bank *Bank, index, tick int) (Transform, error) {
	i, err := FrameIndexAt(bank, index, tick)
	if err != nil {
		return Transform{}, err
	}
	return bank.Animations[index].Frames[i].Transform, nil
}

// Period reports an animation's loop period and intro length in ticks.
func Period(bank *Bank, index int) (period, intro int) {
	if bank == nil || index < 0 || index >= len(bank.Animations) {
		return 1, 0
	}
	a := &bank.Animations[index]
	n := len(a.Frames)
	loopStart := a.LoopStart + 1
	loop := a.Playback == 2 || a.Playback == 4
	ping := a.Playback == 3 || a.Playback == 4
	reverseStart := n - 1
	reverseEnd := 1
	if loop {
		reverseEnd = loopStart
	}
	reverse := 0
	if ping && reverseStart >= reverseEnd {
		reverse = rangeDuration(a, reverseStart-1, reverseEnd-1, -1)
	}
	if loop {
		return max(1, rangeDuration(a, loopStart-1, n-1, 1)+reverse), rangeDuration(a, 0, loopStart-2, 1)
	}
	return 1, max(0, rangeDuration(a, 0, n-1, 1)+reverse-1)
}

// Idle picks the map index of the "stay" animation, falling back to the first.
func Idle(nmar *Bank) (mapIndex, animIndex int, err error) {
	if nmar == nil || len(nmar.Animations) == 0 {
		return 0, 0, fmt.Errorf("animation bank is empty")
	}
	for i, a := range nmar.Animations {
		if a.Label == "stay" {
			return a.Frames[0].CellID, i, nil
		}
	}
	return nmar.Animations[0].Frames[0].CellID, 0, nil
}
