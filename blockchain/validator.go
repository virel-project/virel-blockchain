package blockchain

import "github.com/virel-project/virel-blockchain/p2p"

// Validator implements a thread pool used for efficient block validation
type Validator struct {
	bc          *Blockchain
	Parallelism int
	newBlocks   chan p2p.Packet
}

func (bc *Blockchain) NewValidator(parallelism int) *Validator {
	v := &Validator{
		bc:          bc,
		Parallelism: parallelism,
		newBlocks:   make(chan p2p.Packet, parallelism),
	}

	v.startProcessingBlocks()

	return v
}

func (v *Validator) Close() {
	close(v.newBlocks)
}

func (v *Validator) startProcessingBlocks() {
	for range v.Parallelism {
		go func() {
			for {
				blpacket, ok := <-v.newBlocks
				if !ok {
					return
				}
				v.bc.packetBlock(blpacket)
			}
		}()
	}
}

func (v *Validator) ProcessBlock(blpacket p2p.Packet) {
	v.newBlocks <- blpacket
}
