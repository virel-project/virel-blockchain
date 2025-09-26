//go:build !testnet && !unittest

package checkpoints

import (
	_ "embed"
	"encoding/binary"
)

//go:embed checkpoints.bin
var bin []byte

// checkpoints generated with: create_checkpoints 226297
const CHECKPOINTS_BLAKE3 = "fee8fd84a254f8694fe1b04b112389e3765b7fec90e637e7928550e561f487d7"

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
