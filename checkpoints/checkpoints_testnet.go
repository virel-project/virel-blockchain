//go:build testnet || unittest

package checkpoints

func GetCheckpoint(height uint64) [32]byte {
	return [32]byte{}
}

func IsCheckpoint(height uint64) bool {
	return false
}

func IsSecured(height uint64) bool {
	return false
}
