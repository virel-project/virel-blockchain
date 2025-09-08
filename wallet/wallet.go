// package wallet provides a wallet API for the virel-blockchain network

package wallet

import (
	"errors"
	"fmt"
	"os"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/blockchain"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/rpc/daemonrpc"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
)

// wallet is not concurrency-safe, it should be used on a single thread
type Wallet struct {
	dbInfo dbInfo

	rpc *daemonrpc.RpcClient

	height        uint64
	balance       uint64
	lastNonce     uint64
	mempoolBal    uint64
	mempoolNonce  uint64
	delegateId    uint64
	delegateName  string
	stakedBalance uint64
	totalStaked   uint64
	totalUnstaked uint64

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

	entropy := make([]byte, SEED_ENTROPY)
	bitcrypto.RandRead(entropy)
	w.dbInfo.Mnemonic, w.dbInfo.PrivateKey = newMnemonic(entropy)

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
		Address: w.dbInfo.Address.String(),
	})
	if err != nil {
		return err
	}
	w.balance = res.Balance
	w.lastNonce = res.LastNonce
	w.mempoolBal = res.MempoolBalance
	w.mempoolNonce = res.MempoolNonce
	w.delegateId = res.DelegateId
	w.totalStaked = res.TotalStaked
	w.totalUnstaked = res.TotalUnstaked
	w.height = res.Height

	if res.DelegateId != 0 {
		delegateRes, err := w.rpc.GetDelegate(daemonrpc.GetDelegateRequest{
			DelegateId: w.delegateId,
		})
		if err != nil {
			return fmt.Errorf("could not get delegate: %w", err)
		}
		w.delegateName = delegateRes.Name
		if len(w.delegateName) > 20 {
			w.delegateName = w.delegateName[:20] + "..."
		}

		for _, v := range delegateRes.Funds {
			if v.Owner == w.GetAddress().Addr {
				w.stakedBalance = v.Amount
				break
			}
		}
	} else {
		w.stakedBalance = 0
		w.delegateName = ""
	}

	return nil
}

// Only for unit tests
func (w *Wallet) ManualRefresh(state *blockchain.State, height uint64) {
	w.balance = state.Balance
	w.lastNonce = state.LastNonce
	w.mempoolBal = state.Balance
	w.mempoolNonce = state.LastNonce
	w.delegateId = state.DelegateId
	w.totalStaked = state.TotalStaked
	w.totalUnstaked = state.TotalUnstaked
	w.height = height
}

func (w *Wallet) Rpc() *daemonrpc.RpcClient {
	return w.rpc
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
func (w *Wallet) GetDelegateId() uint64 {
	return w.delegateId
}
func (w *Wallet) GetDelegateName() string {
	return w.delegateName
}
func (w *Wallet) GetStakedBalance() uint64 {
	return w.stakedBalance
}
func (w *Wallet) GetTotalStaked() uint64 {
	return w.totalStaked
}
func (w *Wallet) GetTotalUnstaked() uint64 {
	return w.totalUnstaked
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

func (w *Wallet) checkAndSignTx(tx *transaction.Transaction) error {
	tx.Fee = tx.GetVirtualSize() * config.FEE_PER_BYTE_V2

	addr := w.GetAddress().Addr

	var amt uint64
	for _, v := range tx.Data.StateInputs(tx, addr) {
		if v.Sender == addr {
			amt += v.Amount
		}
	}

	if amt > w.mempoolBal {
		return errors.New("transaction spends too much money")
	}

	return tx.Sign(w.dbInfo.PrivateKey)
}

// This method doesn't submit the transaction. Use the SubmitTx method to submit it to the network.
func (w *Wallet) Transfer(outputs []transaction.Output, hasVersion bool) (*transaction.Transaction, error) {
	for _, out := range outputs {
		if w.GetAddress().Addr == out.Recipient {
			return nil, fmt.Errorf("cannot transfer funds to self")
		}
	}

	for i := 0; i < len(outputs); i++ {
		for j := i + 1; j < len(outputs); j++ {
			if outputs[i].Recipient == outputs[j].Recipient && outputs[i].PaymentId == outputs[j].PaymentId {
				outputs[i].Amount += outputs[j].Amount
				// Remove the duplicate by slicing
				outputs = append(outputs[:j], outputs[j+1:]...)
				j-- // Adjust index since we removed an element
			}
		}
	}

	txn := &transaction.Transaction{
		Signer: w.dbInfo.PrivateKey.Public(),
		Nonce:  w.GetMempoolLastNonce() + 1,
		Data: &transaction.Transfer{
			Outputs: outputs,
		},
	}

	if hasVersion {
		txn.Version = 1
	}

	return txn, w.checkAndSignTx(txn)
}

// This method doesn't submit the transaction. Use the SubmitTx method to submit it to the network.
func (w *Wallet) RegisterDelegate(name string, id uint64) (*transaction.Transaction, error) {
	txn := &transaction.Transaction{
		Version: transaction.TX_VERSION_REGISTER_DELEGATE,
		Signer:  w.dbInfo.PrivateKey.Public(),
		Nonce:   w.GetMempoolLastNonce() + 1,
		Data: &transaction.RegisterDelegate{
			Name: []byte(name),
			Id:   id,
		},
	}

	return txn, w.checkAndSignTx(txn)
}

// This method doesn't submit the transaction. Use the SubmitTx method to submit it to the network.
func (w *Wallet) SetDelegate(delegateId, previousId uint64) (*transaction.Transaction, error) {
	txn := &transaction.Transaction{
		Version: transaction.TX_VERSION_SET_DELEGATE,
		Signer:  w.dbInfo.PrivateKey.Public(),
		Nonce:   w.GetMempoolLastNonce() + 1,
		Data: &transaction.SetDelegate{
			DelegateId:       delegateId,
			PreviousDelegate: previousId,
		},
	}

	return txn, w.checkAndSignTx(txn)
}

// This method doesn't submit the transaction. Use the SubmitTx method to submit it to the network.
func (w *Wallet) Stake(delegateId, amount, prevUnlock uint64) (*transaction.Transaction, error) {
	txn := &transaction.Transaction{
		Version: transaction.TX_VERSION_STAKE,
		Signer:  w.dbInfo.PrivateKey.Public(),
		Nonce:   w.GetMempoolLastNonce() + 1,
		Data: &transaction.Stake{
			Amount:     amount,
			DelegateId: delegateId,
			PrevUnlock: prevUnlock,
		},
	}

	return txn, w.checkAndSignTx(txn)
}

// This method doesn't submit the transaction. Use the SubmitTx method to submit it to the network.
func (w *Wallet) Unstake(delegateId uint64, amount uint64) (*transaction.Transaction, error) {
	txn := &transaction.Transaction{
		Version: transaction.TX_VERSION_UNSTAKE,
		Signer:  w.dbInfo.PrivateKey.Public(),
		Nonce:   w.GetMempoolLastNonce() + 1,
		Data: &transaction.Unstake{
			Amount:     amount,
			DelegateId: delegateId,
		},
	}

	return txn, w.checkAndSignTx(txn)
}

func (w *Wallet) SubmitTx(txn *transaction.Transaction) (*daemonrpc.SubmitTransactionResponse, error) {
	return w.rpc.SubmitTransaction(daemonrpc.SubmitTransactionRequest{
		Hex: txn.Serialize(),
	})
}

func (w *Wallet) SignBlockHash(hash util.Hash) (bitcrypto.Signature, error) {
	return bitcrypto.Sign(append(config.STAKE_SIGN_PREFIX, hash[:]...), w.dbInfo.PrivateKey)
}
