//go:build !testnet && !unittest

package checkpoints

import (
	_ "embed"
	"encoding/binary"
)

//go:embed checkpoints.bin
var bin []byte

// checkpoints generated with: create_checkpoints 292867
const CHECKPOINTS_BLAKE3 = "34b8ea64b55976b14a88b9037572f16dbd90c0c87f91c01912ea77307d75b5f5"

const checkpoint_overhead = 4

var CheckpointInterval uint64
var MaxCheckpoint uint64

func init() {
	if len(bin) == 0 {
		return
	}
	CheckpointInterval = uint64(binary.LittleEndian.Uint32(bin))
	if CheckpointInterval < 1 {
		panic("invalid checkpoint interval")
	}

	MaxCheckpoint = uint64((len(bin) - checkpoint_overhead) / 32)
}

func GetCheckpoint(height uint64) [32]byte {
	index := height/CheckpointInterval - 1
	startPos := checkpoint_overhead + (index * 32)

	return [32]byte(bin[startPos : startPos+32])
}

// returns true if the given height is directly checkpointed
func IsCheckpoint(height uint64) bool {
	if MaxCheckpoint == 0 {
		return false
	}
	return height%CheckpointInterval == 0 && height/CheckpointInterval <= MaxCheckpoint
}

// returns true if the given height is directly or indirectly checkpointed
func IsSecured(height uint64) bool {
	if MaxCheckpoint == 0 {
		return false
	}
	return height/CheckpointInterval <= MaxCheckpoint
}
