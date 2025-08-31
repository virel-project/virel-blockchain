package transaction

import (
	"fmt"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
)

type Output struct {
	Recipient address.Address `json:"recipient"`  // recipient's address
	PaymentId uint64          `json:"payment_id"` // subaddress id
	Amount    uint64          `json:"amount"`     // amount excludes the fee
}

func (o Output) Serialize(b *binary.Ser) {
	b.AddFixedByteArray(o.Recipient[:])
	b.AddUvarint(o.PaymentId)
	b.AddUvarint(o.Amount)
}
func (o *Output) Deserialize(d *binary.Des) error {
	o.Recipient = address.Address(d.ReadFixedByteArray(address.SIZE))
	o.PaymentId = d.ReadUvarint()
	o.Amount = d.ReadUvarint()

	return d.Error()
}
func (o Output) String() string {
	return fmt.Sprintf("amount: %d recipient: %v subaddr: %d", o.Amount, o.Recipient, o.PaymentId)
}

type StateInputType uint8

const (
	IN_NORMAL  StateInputType = iota
	IN_UNSTAKE                // This input comes from a delegate for unstaking
)

type StateInput struct {
	Amount    uint64          `json:"amount"`
	Sender    address.Address `json:"sender"`
	Type      StateInputType  `json:"type"`
	ExtraData uint64          `json:"extra_data"`
}

func (o StateInput) String() string {
	return fmt.Sprintf("amount: %d sender: %v", o.Amount, o.Sender)
}

type StateOutputType uint8

const (
	OUT_NORMAL StateOutputType = iota
	OUT_COINBASE_DEV
	OUT_COINBASE_POW
	OUT_COINBASE_POS
	OUT_COINBASE_BURN
	OUT_STAKE // This output goes to a delegate for staking
)

type StateOutput struct {
	Type      StateOutputType `json:"type"`       //
	Amount    uint64          `json:"amount"`     // amount excludes the fee
	Recipient address.Address `json:"recipient"`  // recipient's address
	PaymentId uint64          `json:"payment_id"` // subaddress id
	ExtraData uint64          `json:"extra_data"`
}

func (o StateOutput) String() string {
	return fmt.Sprintf("type: %d amount: %d recipient: %v payid: %d extradata: %d", o.Type, o.Amount, o.Recipient, o.PaymentId, o.ExtraData)
}
