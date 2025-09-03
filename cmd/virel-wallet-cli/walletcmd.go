package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/wallet"

	"github.com/ergochat/readline"
)

type Cmd struct {
	Names  []string
	Action func(args []string)
	Args   string
}

var commands = Commands{}

type Commands []Cmd

// Readline will pass the whole line and current offset to it
// Completer need to pass all the candidates, and how long they shared the same characters in line
// Example:
//
// [go, git, git-shell, grep]
// Do("g", 1) => ["o", "it", "it-shell", "rep"], 1
// Do("gi", 2) => ["t", "t-shell"], 2
// Do("git", 3) => ["", "-shell"], 3
func (c Commands) Do(line []rune, pos int) (newLine [][]rune, length int) {
	if len(line) == 0 {
		return [][]rune{}, 0
	}

	lineStr := string(line)

	sols := [][]rune{}

	for _, v := range c {
		//for i := range v.Names {
		i := 0
		if len(v.Names[i]) >= len(lineStr) {
			sol := v.Names[i][:len(lineStr)]

			if lineStr == sol {
				sols = append(sols, []rune(v.Names[i][len(lineStr):]))
			}
		}
		//}
	}

	return sols, pos
}

func prompts(w *wallet.Wallet) {
	l, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[32m>\033[0m ",
		AutoComplete:    commands,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold: true,
	})
	if err != nil {
		panic(err)
	}
	defer l.Close()

	Log.SetStdout(l.Stdout())
	Log.SetStderr(l.Stderr())

	commands = append(commands, []Cmd{{
		Names: []string{"status", "info", "balance", "addr", "address"},
		Args:  "",
		Action: func(args []string) {
			err := w.Refresh()
			if err != nil {
				Log.Warn("refresh failed:", err)
			}
			Log.Infof("Wallet %s", w.GetAddress())
			Log.Infof("Balance: %s", util.FormatCoin(w.GetBalance()))
			Log.Infof("Last nonce: %d", w.GetLastNonce())
			if w.GetDelegateId() != 0 {
				Log.Infof("Delegate: %s (%s)", w.GetDelegateName(), address.NewDelegateAddress(w.GetDelegateId()))
				Log.Infof("Staked balance: %s", util.FormatCoin(w.GetStakedBalance()))
			}
		},
	}, {
		Names: []string{"exit", "quit"},
		Args:  "",
		Action: func(args []string) {
			os.Exit(0)
		},
	}, {
		Names: []string{"help"},
		Args:  "",
		Action: func(args []string) {
			Log.Info("List of available commands:")
			for _, v := range commands {
				Log.Infof("%s %s", util.PadR(v.Names[0], 14), v.Args)
			}
		},
	}, {
		Names: []string{"register_delegate", "registerdelegate"},
		Args:  "<delegate id or address> <delegate name>",
		Action: func(args []string) {
			const USAGE = "Usage: register_delegate <delegate id or address> <delegate name>"
			if len(args) < 1 {
				Log.Err(USAGE)
				return
			}

			delegateStr := args[0]
			delegateStr = strings.TrimPrefix(delegateStr, config.DELEGATE_ADDRESS_PREFIX)
			delegateId, err := strconv.ParseUint(delegateStr, 10, 64)
			if err != nil {
				Log.Err("invalid delegate id:", err)
				return
			}

			name := strings.Join(args[1:], " ")

			err = w.Refresh()
			if err != nil {
				Log.Err("failed to refresh wallet:", err)
				return
			}

			txn, err := w.RegisterDelegate(name, delegateId)
			if err != nil {
				Log.Err(err)
				return
			}
			totalamt, err := txn.TotalAmount()
			if err != nil {
				Log.Err(err)
				return
			}
			if totalamt+txn.Fee > w.GetBalance() {
				Log.Err("not enough money")
				return
			}
			Log.Infof("Delegate name: %s", name)
			Log.Infof("Delegate id: %s", name)
			Log.Infof("Fee: %v", util.FormatCoin(txn.Fee))
			Log.Infof("Total spent: %v", util.FormatCoin(totalamt))

			lcfg := l.GeneratePasswordConfig()
			lcfg.MaskRune = '*'

			Log.Prompt("Are you sure you want to submit the transaction? (Y/n)")
			line, err := l.ReadLine()
			if err != nil || !strings.HasPrefix(strings.ToLower(line), "y") {
				Log.Info("Cancelled.")
				return
			}

			submitRes, err := w.SubmitTx(txn)
			if err != nil {
				Log.Err(err)
				return
			}

			Log.Infof("transaction has been submit, txid: %s", submitRes.TXID.String())
		},
	}, {
		Names: []string{"set_delegate", "setdelegate"},
		Args:  "<delegate id or address>",
		Action: func(args []string) {
			const USAGE = "Usage: set_delegate <delegate id or address>"
			if len(args) < 1 {
				Log.Err(USAGE)
				return
			}

			delegateStr := args[0]
			delegateStr = strings.TrimPrefix(delegateStr, config.DELEGATE_ADDRESS_PREFIX)
			delegateId, err := strconv.ParseUint(delegateStr, 10, 64)
			if err != nil {
				Log.Err("invalid delegate id:", err)
				return
			}

			err = w.Refresh()
			if err != nil {
				Log.Err("failed to refresh wallet:", err)
				return
			}

			txn, err := w.SetDelegate(delegateId, w.GetDelegateId())
			if err != nil {
				Log.Err(err)
				return
			}
			totalamt, err := txn.TotalAmount()
			if err != nil {
				Log.Err(err)
				return
			}
			if totalamt+txn.Fee > w.GetBalance() {
				Log.Err("not enough money")
				return
			}
			Log.Infof("Fee: %v", util.FormatCoin(txn.Fee))
			Log.Infof("Total spent: %v", util.FormatCoin(totalamt))

			lcfg := l.GeneratePasswordConfig()
			lcfg.MaskRune = '*'

			Log.Prompt("Are you sure you want to submit the transaction? (Y/n)")
			line, err := l.ReadLine()
			if err != nil || !strings.HasPrefix(strings.ToLower(line), "y") {
				Log.Info("Cancelled.")
				return
			}

			submitRes, err := w.SubmitTx(txn)
			if err != nil {
				Log.Err(err)
				return
			}

			Log.Infof("transaction has been submit, txid: %s", submitRes.TXID.String())
		},
	}, {
		Names: []string{"stake"},
		Args:  "<amount>",
		Action: func(args []string) {
			const USAGE = "Usage: stake <amount>"
			if len(args) < 1 {
				Log.Err(USAGE)
				return
			}

			amtStr := args[0]
			amtFloat, err := strconv.ParseFloat(amtStr, 64)
			if err != nil || amtFloat <= 0 {
				Log.Err("failed to parse amount:", err)
				return
			}

			amt := uint64(amtFloat * config.COIN)

			if amt < config.MIN_STAKE_AMOUNT {
				Log.Err("stake amount is too small, you must stake at least", config.MIN_STAKE_AMOUNT/config.COIN)
				return
			}

			err = w.Refresh()
			if err != nil {
				Log.Err("failed to refresh wallet:", err)
				return
			}

			txn, err := w.Stake(w.GetDelegateId(), amt)
			if err != nil {
				Log.Err(err)
				return
			}
			totalamt, err := txn.TotalAmount()
			if err != nil {
				Log.Err(err)
				return
			}
			if totalamt+txn.Fee > w.GetBalance() {
				Log.Err("not enough money")
				return
			}
			Log.Infof("Fee: %v", util.FormatCoin(txn.Fee))
			Log.Infof("Total spent: %v", util.FormatCoin(totalamt))

			lcfg := l.GeneratePasswordConfig()
			lcfg.MaskRune = '*'

			Log.Prompt("Are you sure you want to submit the transaction? (Y/n)")
			line, err := l.ReadLine()
			if err != nil || !strings.HasPrefix(strings.ToLower(line), "y") {
				Log.Info("Cancelled.")
				return
			}

			submitRes, err := w.SubmitTx(txn)
			if err != nil {
				Log.Err(err)
				return
			}

			Log.Infof("transaction has been submit, txid: %s", submitRes.TXID.String())
		},
	}, {
		Names: []string{"unstake"},
		Args:  "<amount>",
		Action: func(args []string) {
			const USAGE = "Usage: unstake <amount>"
			if len(args) < 1 {
				Log.Err(USAGE)
				return
			}

			amtStr := args[0]
			amtFloat, err := strconv.ParseFloat(amtStr, 64)
			if err != nil || amtFloat <= 0 {
				Log.Err("failed to parse amount:", err)
				return
			}

			amt := uint64(amtFloat * config.COIN)

			err = w.Refresh()
			if err != nil {
				Log.Err("failed to refresh wallet:", err)
				return
			}

			if amt > w.GetStakedBalance() {
				Log.Err("trying to stake more than staked balance:", util.FormatCoin(w.GetStakedBalance()))
				return
			}

			txn, err := w.Unstake(w.GetDelegateId(), amt)
			if err != nil {
				Log.Err(err)
				return
			}
			totalamt, err := txn.TotalAmount()
			if err != nil {
				Log.Err(err)
				return
			}
			Log.Infof("Fee: %v", util.FormatCoin(txn.Fee))
			Log.Infof("Total spent: %v", util.FormatCoin(totalamt))

			lcfg := l.GeneratePasswordConfig()
			lcfg.MaskRune = '*'

			Log.Prompt("Are you sure you want to submit the transaction? (Y/n)")
			line, err := l.ReadLine()
			if err != nil || !strings.HasPrefix(strings.ToLower(line), "y") {
				Log.Info("Cancelled.")
				return
			}

			submitRes, err := w.SubmitTx(txn)
			if err != nil {
				Log.Err(err)
				return
			}

			Log.Infof("transaction has been submit, txid: %s", submitRes.TXID.String())
		},
	}, {
		Names: []string{"transfer"},
		Args:  "<destination> <amount> [<destination 2>] [<amount 2>]",
		Action: func(args []string) {
			const USAGE = "Usage: transfer <destination> <amount> [<destination 2>] [<amount 2>]"
			if len(args) < 2 {
				Log.Err(USAGE)
				return
			}

			outputs := []transaction.Output{}
			var totalAmt uint64 = 0

			for len(args) >= 2 {
				dst, err := address.FromString(args[0])
				if err != nil {
					Log.Err("invalid destination:", err)
					return
				}

				xbal, err := strconv.ParseFloat(args[1], 64)
				if err != nil {
					Log.Err("invalid amount:", err)
					return
				}

				var amt = int64(xbal * config.COIN)

				if amt < 1 {
					Log.Errf("invalid output amount: %d", amt)
					return
				}
				totalAmt += uint64(amt)
				balance := w.GetBalance()
				if totalAmt > balance {
					Log.Errf("transaction spends too much money (%s), wallet balance is: %s", util.FormatCoin(balance))
					return
				}

				outputs = append(outputs, transaction.Output{
					Amount:    uint64(amt),
					Recipient: dst.Addr,
					PaymentId: dst.PaymentId,
				})

				args = args[2:]
			}

			if len(outputs) == 0 {
				Log.Err(USAGE)
				return
			}

			for _, v := range outputs {
				Log.Infof("%v receives %v", v.Recipient, util.FormatCoin(v.Amount))
			}

			err = w.Refresh()
			if err != nil {
				Log.Err("failed to refresh wallet:", err)
				return
			}

			txn, err := w.Transfer(outputs, w.GetHeight() >= config.HARDFORK_V2_HEIGHT)
			if err != nil {
				Log.Err(err)
				return
			}
			Log.Infof("Fee: %v", util.FormatCoin(txn.Fee))
			Log.Infof("Total spent: %v", util.FormatCoin(totalAmt))

			lcfg := l.GeneratePasswordConfig()
			lcfg.MaskRune = '*'

			Log.Prompt("Are you sure you want to submit the transaction? (Y/n)")
			line, err := l.ReadLine()
			if err != nil || !strings.HasPrefix(strings.ToLower(line), "y") {
				Log.Info("Cancelled.")
				return
			}

			submitRes, err := w.SubmitTx(txn)
			if err != nil {
				Log.Err(err)
				return
			}

			Log.Infof("transaction has been submit, txid: %s", submitRes.TXID.String())
		},
	}, {
		Names: []string{"transfer"},
		Args:  "<destination> <amount> [<destination 2>] [<amount 2>]",
		Action: func(args []string) {
			const USAGE = "Usage: transfer <destination> <amount> [<destination 2>] [<amount 2>]"
			if len(args) < 2 {
				Log.Err(USAGE)
				return
			}

			outputs := []transaction.Output{}
			var totalAmt uint64 = 0

			for len(args) >= 2 {
				dst, err := address.FromString(args[0])
				if err != nil {
					Log.Err("invalid destination:", err)
					return
				}

				xbal, err := strconv.ParseFloat(args[1], 64)
				if err != nil {
					Log.Err("invalid amount:", err)
					return
				}

				var amt = int64(xbal * config.COIN)

				if amt < 1 {
					Log.Errf("invalid output amount: %d", amt)
					return
				}
				totalAmt += uint64(amt)
				balance := w.GetBalance()
				if totalAmt > balance {
					Log.Errf("transaction spends too much money (%s), wallet balance is: %s", util.FormatCoin(balance))
					return
				}

				outputs = append(outputs, transaction.Output{
					Amount:    uint64(amt),
					Recipient: dst.Addr,
					PaymentId: dst.PaymentId,
				})

				args = args[2:]
			}

			if len(outputs) == 0 {
				Log.Err(USAGE)
				return
			}

			for _, v := range outputs {
				Log.Infof("%v receives %v", v.Recipient, util.FormatCoin(v.Amount))
			}

			err = w.Refresh()
			if err != nil {
				Log.Err("failed to refresh wallet:", err)
				return
			}

			txn, err := w.Transfer(outputs, w.GetHeight() >= config.HARDFORK_V2_HEIGHT)
			if err != nil {
				Log.Err(err)
				return
			}
			Log.Infof("Fee: %v", util.FormatCoin(txn.Fee))
			Log.Infof("Total spent: %v", util.FormatCoin(totalAmt))

			lcfg := l.GeneratePasswordConfig()
			lcfg.MaskRune = '*'

			Log.Prompt("Are you sure you want to submit the transaction? (Y/n)")
			line, err := l.ReadLine()
			if err != nil || !strings.HasPrefix(strings.ToLower(line), "y") {
				Log.Info("Cancelled.")
				return
			}

			submitRes, err := w.SubmitTx(txn)
			if err != nil {
				Log.Err(err)
				return
			}

			Log.Infof("transaction has been submit, txid: %s", submitRes.TXID.String())
		},
	}, {
		Names: []string{"list", "list_transactions", "list_tx", "list_txs"},
		Args:  "",
		Action: func(args []string) {
			var page uint64
			if len(args) > 0 {
				var err error
				page, err = strconv.ParseUint(args[0], 10, 64)
				if err != nil || page == 0 {
					Log.Warn("invalid page number")
				}
				page--
			}

			txs, err := w.GetTransactions(true, page)
			if err != nil {
				Log.Warn("failed to get transactions:", err)
				return
			}
			Log.Infof("Incoming transactions (%d)", len(txs.Transactions))
			for _, v := range txs.Transactions {
				Log.Infof(" - %s", v)
			}
			maxPage := txs.MaxPage
			txs, err = w.GetTransactions(false, page)
			if err != nil {
				Log.Warn("failed to get transactions:", err)
				return
			}
			Log.Infof("Outgoing transactions (%d)", len(txs.Transactions))
			for _, v := range txs.Transactions {
				Log.Infof(" - %s", v)
			}
			maxPage = max(maxPage, txs.MaxPage)

			Log.Infof("page %d/%d", page+1, maxPage+1)
		},
	}, {
		Names: []string{"seedphrase", "seed", "mnemonic"},
		Args:  "",
		Action: func(args []string) {
			Log.Infof("Your mnemonic seed phrase:")
			Log.Infof(w.GetMnemonic())
			Log.Infof("Write down this mnemonic seed phrase on a secure offline medium (e.g. paper) to recover your wallet.")
		},
	}}...)

	for {
		line, err := l.ReadLine()
		if err != nil {
			Log.Err(err)
			os.Exit(0)
		}

		args := strings.Split(line, " ")

		if len(args) == 0 {
			continue
		}

		executed := false
		for _, v := range commands {
			for _, v2 := range v.Names {
				if v2 == args[0] {
					v.Action(args[1:])
					executed = true
					break
				}
			}
			if executed {
				break
			}
		}

		if !executed {
			Log.Err("command", args[0], "not found")
		}
	}
}
