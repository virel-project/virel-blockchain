// package wallet provides a wallet API for the virel-blockchain network

package wallet

import (
	"errors"
	"fmt"
	"os"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/bitcrypto"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/rpc/daemonrpc"
	"github.com/virel-project/virel-blockchain/transaction"
	"github.com/virel-project/virel-blockchain/util"
)

// wallet is not concurrency-safe, it should be used on a single thread
type Wallet struct {
	dbInfo dbInfo

	rpc *daemonrpc.RpcClient

	height       uint64
	balance      uint64
	lastNonce    uint64
	mempoolBal   uint64
	mempoolNonce uint64

	password []byte
}

type dbInfo struct {
	NetworkID  uint64
	Mnemonic   string
	PrivateKey bitcrypto.Privkey
	Address    address.Integrated
}

func OpenWallet(rpcAddr string, walletdb, pass []byte) (*Wallet, error) {
	w := &Wallet{
		rpc:      daemonrpc.NewRpcClient(rpcAddr),
		balance:  0,
		password: pass,
	}

	return w, w.decodeDatabase(walletdb, pass)
}
func OpenWalletFile(rpcAddr, filename string, pass []byte) (*Wallet, error) {
	walletdb, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return OpenWallet(rpcAddr, walletdb, pass)
}

func CreateWallet(rpcAddr string, pass []byte, fastkdf bool) (*Wallet, []byte, error) {
	w := &Wallet{
		rpc:      daemonrpc.NewRpcClient(rpcAddr),
		balance:  0,
		dbInfo:   dbInfo{},
		password: pass,
	}

	w.dbInfo.Mnemonic, w.dbInfo.PrivateKey = newMnemonic()

	w.dbInfo.Address = address.FromPubKey(w.dbInfo.PrivateKey.Public()).Integrated()

	var kdfMemory uint32 = 6
	var kdfIterations uint32 = 512
	if fastkdf {
		kdfMemory = 2
		kdfIterations = 128
	}

	dbEnc, err := saveDatabase(w.dbInfo, pass, kdfIterations, kdfMemory*1024)

	if err != nil {
		return nil, dbEnc, err
	}

	return w, dbEnc, nil
}

func CreateWalletFile(rpcAddr, filename string, pass []byte) (*Wallet, error) {
	_, err := os.Lstat(filename)
	if err == nil {
		return nil, errors.New("wallet already exists")
	}

	wall, dbEnc, err := CreateWallet(rpcAddr, pass, false)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(filename, dbEnc, 0o660)
	return wall, err
}

func CreateWalletFromMnemonic(
	rpcAddr, mnemonic string, pass []byte, fastkdf bool,
) (*Wallet, []byte, error) {
	w := &Wallet{
		rpc:      daemonrpc.NewRpcClient(rpcAddr),
		balance:  0,
		dbInfo:   dbInfo{},
		password: pass,
	}
	var err error
	w.dbInfo.PrivateKey, err = decodeMnemonic(mnemonic)
	if err != nil {
		return nil, nil, err
	}
	w.dbInfo.Mnemonic = mnemonic
	w.dbInfo.Address = address.FromPubKey(w.dbInfo.PrivateKey.Public()).Integrated()

	var kdfMemory uint32 = 6
	var kdfIterations uint32 = 512
	if fastkdf {
		kdfMemory = 2
		kdfIterations = 128
	}

	dbEnc, err := saveDatabase(w.dbInfo, pass, kdfIterations, kdfMemory*1024)

	return w, dbEnc, err
}

func CreateWalletFileFromMnemonic(
	rpcAddr, filename, mnemonic string, pass []byte,
) (*Wallet, error) {
	_, err := os.Lstat(filename)
	if err == nil {
		return nil, errors.New("wallet already exists")
	}

	wall, dbEnc, err := CreateWalletFromMnemonic(rpcAddr, mnemonic, pass, false)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(filename, dbEnc, 0o660)
	return wall, err
}

func (w *Wallet) Refresh() error {
	if w.rpc == nil {
		return errors.New("w.rpc is nil")
	}

	res, err := w.rpc.GetAddress(daemonrpc.GetAddressRequest{
		Address: w.dbInfo.Address,
	})
	if err != nil {
		return err
	}
	w.balance = res.Balance
	w.lastNonce = res.LastNonce
	w.mempoolBal = res.MempoolBalance
	w.mempoolNonce = res.MempoolNonce
	w.height = res.Height

	return nil
}

func (w *Wallet) GetPassword() []byte {
	return w.password
}
func (w *Wallet) GetHeight() uint64 {
	return w.height
}
func (w *Wallet) GetBalance() uint64 {
	return w.balance
}
func (w *Wallet) GetLastNonce() uint64 {
	return w.lastNonce
}
func (w *Wallet) GetMempoolBalance() uint64 {
	return w.mempoolBal
}
func (w *Wallet) GetMempoolLastNonce() uint64 {
	return w.mempoolNonce
}
func (w *Wallet) GetAddress() address.Integrated {
	return w.dbInfo.Address
}
func (w *Wallet) GetTransactions(inc bool, page uint64) (*daemonrpc.GetTxListResponse, error) {
	r := daemonrpc.GetTxListRequest{
		Address: w.GetAddress(),
		Page:    page,
	}

	if inc {
		r.TransferType = "incoming"
	} else {
		r.TransferType = "outgoing"
	}

	return w.rpc.GetTxList(r)
}
func (w *Wallet) GetMnemonic() string {
	return w.dbInfo.Mnemonic
}

func (w *Wallet) GetTransaction(txid util.Hash) (*daemonrpc.GetTransactionResponse, error) {
	return w.rpc.GetTransaction(daemonrpc.GetTransactionRequest{
		Txid: txid,
	})
}

func (w *Wallet) GetRpcDaemonAddress() string {
	return w.rpc.DaemonAddress
}
func (w *Wallet) SetRpcDaemonAddress(a string) {
	w.rpc.DaemonAddress = a
}

// This method doesn't submit the transaction. Use the SubmitTx method to submit it to the network.
func (w *Wallet) Transfer(amount uint64, recipient address.Integrated) (*transaction.Transaction, error) {
	err := w.Refresh()
	if err != nil {
		return nil, fmt.Errorf("wallet is not connected to daemon: %w", err)
	}

	if w.GetAddress() == recipient {
		return nil, fmt.Errorf("cannot transfer funds to self")
	}

	txn := &transaction.Transaction{
		Sender: w.dbInfo.PrivateKey.Public(),
		Nonce:  w.GetMempoolLastNonce() + 1,
		Outputs: []transaction.Output{
			{
				Recipient: recipient.Addr,
				Amount:    amount,
				Subaddr:   recipient.Subaddr,
			},
		},
		Fee: 0,
	}

	txn.Fee = txn.GetVirtualSize() * config.FEE_PER_BYTE

	err = txn.Sign(w.dbInfo.PrivateKey)

	return txn, err
}

func (w *Wallet) SubmitTx(txn *transaction.Transaction) (*daemonrpc.SubmitTransactionResponse, error) {
	return w.rpc.SubmitTransaction(daemonrpc.SubmitTransactionRequest{
		Hex: txn.Serialize(),
	})
}
