package transaction

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/config"
)

const TX_VERSION_TRANSFER = 1
const TX_VERSION_REGISTER_DELEGATE = 2
const TX_VERSION_SET_DELEGATE = 3

type TransactionData interface {
	AssociatedTransactionVersion() uint8
	Serialize(s *binary.Ser)
	Deserialize(d *binary.Des) error
	String() string
	VSize() uint64
	TotalAmount() (uint64, error)
	Prevalidate() error
	// Adds the balances of this transaction data to the blockchain state.
	// The sum of the amounts of this method must be equal to TransactionData's TotalAmount().
	AddToState() []Output
}

// TransactionData: Transfer

type Transfer struct {
	Outputs []Output // transaction outputs
}

func (t *Transfer) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_TRANSFER
}
func (t *Transfer) Serialize(s *binary.Ser) {
	s.AddUvarint(uint64(len(t.Outputs)))
	for _, v := range t.Outputs {
		v.Serialize(s)
	}
}
func (t *Transfer) Deserialize(d *binary.Des) error {
	numOutputs := d.ReadUvarint()
	if numOutputs > config.MAX_OUTPUTS || numOutputs == 0 {
		return errors.New("too many outputs")
	}
	t.Outputs = make([]Output, numOutputs)
	for i, v := range t.Outputs {
		err := v.Deserialize(d)
		if err != nil {
			return err
		}
		t.Outputs[i] = v
	}
	return d.Error()
}
func (t *Transfer) String() string {
	o := "Transfer with outputs:\n"
	for _, v := range t.Outputs {
		o += " - " + v.String() + "\n"
	}
	return o
}
func (t *Transfer) VSize() uint64 {
	return uint64(len(t.Outputs)) * output_overhead
}
func (t *Transfer) TotalAmount() (uint64, error) {
	var s uint64 = 0

	for _, v := range t.Outputs {
		prev := s
		s += v.Amount

		// prevent overflow on outputs
		if prev > s {
			return 0, errors.New("overflow on output")
		}
	}

	return s, nil
}
func (t *Transfer) Prevalidate() error {
	// verify the number of outputs
	if len(t.Outputs) == 0 || len(t.Outputs) > config.MAX_OUTPUTS {
		return fmt.Errorf("invalid output count: %d", len(t.Outputs))
	}

	return nil
}
func (t *Transfer) AddToState() []Output {
	return t.Outputs
}

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

// TransactionData: RegisterDelegate

type RegisterDelegate struct {
	Name []byte
}

func (t *RegisterDelegate) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_REGISTER_DELEGATE
}
func (t *RegisterDelegate) Serialize(s *binary.Ser) {
	s.AddByteSlice(t.Name)
}
func (t *RegisterDelegate) Deserialize(d *binary.Des) error {
	t.Name = d.ReadByteSlice()
	return d.Error()
}
func (t *RegisterDelegate) String() string {
	return "RegisterDelegate " + strconv.Quote(string(t.Name))
}
func (t *RegisterDelegate) TotalAmount() (uint64, error) {
	return 0, nil
}
func (t *RegisterDelegate) VSize() uint64 {
	return 16 + uint64(len(t.Name))
}
func (t *RegisterDelegate) Prevalidate() error {
	if len(t.Name) > 16 {
		return errors.New("RegisterDelegate name is too long")
	}
	return nil
}

func (t *RegisterDelegate) AddToState() []Output {
	return make([]Output, 0)
}

// TransactionData: SetDelegate

type SetDelegate struct {
	DelegateId uint64
}

func (t *SetDelegate) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_SET_DELEGATE
}
func (t *SetDelegate) Serialize(s *binary.Ser) {
	s.AddUvarint(t.DelegateId)
}
func (t *SetDelegate) Deserialize(d *binary.Des) error {
	t.DelegateId = d.ReadUvarint()
	return d.Error()
}
func (t *SetDelegate) String() string {
	return fmt.Sprintf("SetDelegate: %d", t.DelegateId)
}
func (t *SetDelegate) TotalAmount() (uint64, error) {
	return 0, nil
}
func (t *SetDelegate) VSize() uint64 {
	return 4
}
func (t *SetDelegate) Prevalidate() error {
	return nil
}
func (t *SetDelegate) AddToState() []Output {
	return make([]Output, 0)
}
