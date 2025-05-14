package buck

const (
	INFO  = iota // generic blockchain info
	BLOCK        // block hash -> block data
	TOPO         // topology (height -> mainchain block hash)
	STATE        // wallet address -> state content
	TX           // TXID -> tx data
	OUTTX        // wallet address + tx nonce -> outgoing TXID
	INTX         // wallet address + incoming nonce -> incoming TXID
)
