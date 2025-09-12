//go:build !testnet && !unittest

package checkpoints

import (
	_ "embed"
	"encoding/binary"
)

//go:embed checkpoints.bin
var bin []byte

// checkpoints generated with: create_checkpoints 52565
const CHECKPOINTS_BLAKE3 = "d80265e9e3f92390b5b41fda1f563b0d6d4432d394a31ec8bcb02ebda1f5f8d8"

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
