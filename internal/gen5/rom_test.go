package gen5_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bwsprites/internal/export"
	"bwsprites/internal/gen5"
)

const dumpSize = 268435456

// TestIdentifyRejectsWrongSizes covers the failure a user hits with a trimmed
// dump, without needing a real ROM.
func TestIdentifyRejectsWrongSizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short.nds")
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gen5.Identify(path); err == nil || !strings.Contains(err.Error(), "268435456") {
		t.Fatalf("Identify on a short file: %v", err)
	}
}

// TestIdentifyRejectsForeignDumps builds a sparse 256 MiB file, the size a
// Black/White dump has, and checks the game-code check catches a different game.
func TestIdentifyRejectsForeignDumps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foreign.nds")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(dumpSize); err != nil {
		f.Close()
		t.Skipf("cannot make a sparse 256 MiB file here: %v", err)
	}
	// A real dump carries the game code at 0x0C; zeroes are not one.
	if _, err := f.WriteAt([]byte("NOPE"), 0x0C); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if _, err := gen5.Identify(path); err == nil || !strings.Contains(err.Error(), "game code") {
		t.Fatalf("Identify on a foreign dump: %v", err)
	}
}

// TestRealROM round-trips the importer against a dump the user supplies, since
// the sprite data itself can never ship with this project. Set BW_ROM to an
// untrimmed Black or White dump to run it:
//
//	BW_ROM=/path/to/black.nds go test ./internal/gen5
func TestRealROM(t *testing.T) {
	path := os.Getenv("BW_ROM")
	if path == "" {
		t.Skip("set BW_ROM to an untrimmed Black/White dump to check the importer against real data")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	rom, err := gen5.Identify(path)
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if rom.Size() != dumpSize {
		t.Fatalf("size = %d, want %d", rom.Size(), dumpSize)
	}
	t.Logf("dump: %s code %s", rom.Name(), rom.Code())

	// Reading must not touch the source: the size and mtime stay put.
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("the ROM was modified by a read")
	}

	for _, dex := range []int{1, 3, 6, 25, 150, 649} {
		for _, side := range []string{"front", "back"} {
			out, err := export.One(rom, export.Request{Dex: dex, Side: side, Palette: "normal", FrameSize: 0, Ground: -1})
			if err != nil {
				t.Errorf("dex %d %s: %v", dex, side, err)
				continue
			}
			e := out.Entry
			if e.Frames < 1 || e.Frames != len(e.Offsets) || e.Frames != len(e.Durations) {
				t.Errorf("dex %d %s: %d frames, %d offsets, %d durations", dex, side, e.Frames, len(e.Offsets), len(e.Durations))
			}
			sum := 0
			for _, d := range e.Durations {
				if d < 1 {
					t.Errorf("dex %d %s: zero-length frame", dex, side)
				}
				sum += d
			}
			if sum != e.CycleTicks {
				t.Errorf("dex %d %s: durations add up to %d ticks, cycle is %d", dex, side, sum, e.CycleTicks)
			}
			if e.Grounded && e.Union[3]-1 != e.Ground {
				t.Errorf("dex %d %s: grounded but content ends on row %d, ground is %d", dex, side, e.Union[3]-1, e.Ground)
			}
			if e.Union[0] < 0 || e.Union[1] < 0 || e.Union[2] > e.FrameSize || e.Union[3] > e.FrameSize {
				t.Errorf("dex %d %s: union %v outside the %dpx cell", dex, side, e.Union, e.FrameSize)
			}
		}
	}
}
