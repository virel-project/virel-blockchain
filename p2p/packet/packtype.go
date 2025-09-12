package packet

type Type uint16

const (
	PING = iota
	BLOCK
	TX
	STATS
	BLOCK_REQUEST
	STAKE_SIGNATURE
)

func (p Type) String() string {
	switch p {
	case PING:
		return "PING"
	case BLOCK:
		return "BLOCK"
	case TX:
		return "TX"
	case STATS:
		return "STATS"
	case BLOCK_REQUEST:
		return "BLOCK_REQUEST"
	case STAKE_SIGNATURE:
		return "STAKE_SIGNATURE"
	}
	return "UNKNOWN"
}
