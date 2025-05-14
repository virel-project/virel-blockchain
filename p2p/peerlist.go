package p2p

import (
	"encoding/json"
	"os"
	"virel-blockchain/config"
)

func (p *P2P) savePeerlist() error {
	p.RLock()
	defer p.RUnlock()
	d, err := json.Marshal(p.KnownPeers)
	if err != nil {
		return err
	}
	return os.WriteFile("./peerlist-"+config.NETWORK_NAME+".json", d, 0o660)
}
