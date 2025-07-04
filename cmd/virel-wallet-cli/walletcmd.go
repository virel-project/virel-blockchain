package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/util"
	"github.com/virel-project/virel-blockchain/wallet"

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
		Names: []string{"transfer"},
		Args:  "<destination> <amount>",
		Action: func(args []string) {
			fmt.Println("args:", args)
			const USAGE = "Usage: transfer <destination> <amount>"
			if len(args) < 2 {
				Log.Err(USAGE)
				return
			}

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

			var amt = uint64(xbal * config.COIN)

			if amt < 1 || amt > w.GetBalance() {
				Log.Errf("transaction spends too much money, wallet balance is: %s", util.FormatCoin(w.GetBalance()))
				return
			}

			Log.Info("transferring", util.FormatCoin(amt), "to", dst)

			txn, err := w.Transfer(amt, dst)
			if err != nil {
				Log.Err(err)
				return
			}

			Log.Infof("transaction has been generated, fee: %s", util.FormatCoin(txn.Fee))

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
