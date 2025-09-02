//go:build testnet || unittest

package checkpoints

const CHECKPOINTS_BLAKE3 = ""

var CheckpointInterval uint64 = 1
var MaxCheckpoint uint64 = 0
var bin = []byte{}

func GetCheckpoint(height uint64) [32]byte {
	return [32]byte{}
}

func IsCheckpoint(height uint64) bool {
	return false
}

func IsSecured(height uint64) bool {
	return false
}
