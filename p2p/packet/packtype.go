package packet

type Type uint16

const (
	PING = iota
	BLOCK
	TX
	STATS
	BLOCK_REQUEST
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
	}
	return "UNKNOWN"
}
