package checkpoints

import (
	"encoding/hex"
	"testing"

	"github.com/zeebo/blake3"
)

func TestCheckpoints(t *testing.T) {
	t.Logf("Interval: %d; Max: %d", CheckpointInterval, MaxCheckpoint)

	if len(bin) > 0 {
		h := blake3.Sum256(bin)
		if hex.EncodeToString(h[:]) != CHECKPOINTS_BLAKE3 {
			t.Fatalf("hash %x does not match CHECKPOINTS_BLAKE3 %s", h, CHECKPOINTS_BLAKE3)
		}
	}
}
