package gen5

import "fmt"

// Pose is one distinct animation frame: a tight crop of the composed sprite.
type Pose struct {
	Canvas   *Canvas
	Duration int
}

// Timeline is a deduplicated idle animation with its loop metadata.
type Timeline struct {
	Poses     []Pose
	LoopStart int
	Ticks     int
	Capped    bool
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func (s *Sprite) signature(tick int) (string, error) {
	if s.MapIndex < 0 || s.MapIndex >= len(s.NCMR.Maps) {
		return "", fmt.Errorf("missing multicell map %d", s.MapIndex)
	}
	sig := fmt.Sprintf("%d", s.MapIndex)
	for _, r := range s.NCMR.Maps[s.MapIndex].Records {
		// Frame identity, not just its cell: animations that only rotate,
		// scale or translate one cell (leaf sway, squash, hover) otherwise
		// collapse into a single still pose.
		frame, err := FrameIndexAt(s.NANR, r.AnimationIndex, tick)
		if err != nil {
			return "", err
		}
		sig += fmt.Sprintf("/%d", frame)
	}
	return sig, nil
}

// cycleTicks returns the animation length, intro offset and cap flag.
func (s *Sprite) cycleTicks() (ticks, intro int, capped bool, err error) {
	m := s.NCMR.Maps[s.MapIndex]
	period, intro := 1, 0
	for _, r := range m.Records {
		p, i := Period(s.NANR, r.AnimationIndex)
		if len(s.NANR.Animations[r.AnimationIndex].Frames) == 1 {
			p, i = 1, 0
		}
		intro = max(intro, i)
		period = min(MaxTicks+1, period/gcd(period, p)*p)
	}
	total := intro + period
	if intro >= MaxTicks {
		return 0, 0, false, fmt.Errorf("animation introduction exceeds the %d tick export bound", MaxTicks)
	}
	return min(total, MaxTicks), intro, total > MaxTicks, nil
}

// Timeline renders the idle animation with pal, merging runs of identical ticks
// into one pose with a duration, as the reference importer does.
func (s *Sprite) Timeline(pal *Palette) (*Timeline, error) {
	ticks, intro, capped, err := s.cycleTicks()
	if err != nil {
		return nil, err
	}
	tl := &Timeline{Ticks: ticks, Capped: capped, LoopStart: 0}
	last := ""
	for tick := 0; tick < ticks; tick++ {
		sig, err := s.signature(tick)
		if err != nil {
			return nil, err
		}
		if tick == intro {
			tl.LoopStart = len(tl.Poses)
		}
		if len(tl.Poses) > 0 && sig == last && tick != intro {
			tl.Poses[len(tl.Poses)-1].Duration++
			continue
		}
		canvas, err := Render(s.Graphics, pal, s.Cells, s.NANR, s.NCMR, s.MapIndex, tick)
		if err != nil {
			return nil, err
		}
		tl.Poses = append(tl.Poses, Pose{Canvas: canvas, Duration: 1})
		last = sig
	}
	if len(tl.Poses) == 0 {
		return nil, fmt.Errorf("idle animation produced no frames")
	}
	return tl, nil
}
