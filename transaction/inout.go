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

type Input struct {
	Amount uint64          `json:"amount"`
	Sender address.Address `json:"recipient"`
}

func (o Input) Serialize(b *binary.Ser) {
	b.AddFixedByteArray(o.Sender[:])
	b.AddUvarint(o.Amount)
}
func (o *Input) Deserialize(d *binary.Des) error {
	o.Sender = address.Address(d.ReadFixedByteArray(address.SIZE))
	o.Amount = d.ReadUvarint()
	return d.Error()
}
func (o Input) String() string {
	return fmt.Sprintf("amount: %d sender: %v", o.Amount, o.Sender)
}
