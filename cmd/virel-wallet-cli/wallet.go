package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/logger"
	"github.com/virel-project/virel-blockchain/util"
	"github.com/virel-project/virel-blockchain/wallet"

	"github.com/ergochat/readline"
)

var Log = logger.New()

var default_rpc = fmt.Sprintf("http://127.0.0.1:%d", config.RPC_BIND_PORT)

func initialPrompt(daemon_address string) *wallet.Wallet {
	l, err := readline.NewEx(&readline.Config{
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold: true,
	})
	if err != nil {
		panic(err)
	}
	defer l.Close()

	Log.Info("Starting", config.NETWORK_NAME, "CLI wallet")
	if config.NETWORK_NAME != "mainnet" {
		Log.Warn("This is a", strings.ToUpper(config.NETWORK_NAME), "node, only for testing the blockchain.")
		Log.Warn("Be aware that any amount transacted in", config.NETWORK_NAME, "is worthless.")
	}

	lcfg := l.GeneratePasswordConfig()
	lcfg.MaskRune = '*'

	for {
		Log.Info("Available commands:")
		Log.Info("open    Open wallet file")
		Log.Info("create  Creates a new wallet")
		Log.Info("restore Restore a wallet from seedphrase")

		l.SetPrompt("\033[32m>\033[0m ")

		line, err := l.ReadLine()
		if err != nil {
			Log.Err(err)
			os.Exit(0)
		}

		cmds := strings.Split(strings.ToLower(strings.ReplaceAll(line, "  ", " ")), " ")

		if len(cmds) == 0 {
			Log.Err("invalid command")
			continue
		}

		cmd := cmds[0]
		if cmd != "open" && cmd != "create" && cmd != "restore" {
			Log.Err("unknown command")
			continue
		}
		if len(cmds) == 1 {
			l.SetPrompt("Wallet name: ")
			filename, err := l.ReadLine()
			if err != nil {
				Log.Err(err)
				os.Exit(0)
			}
			l.SetPrompt("\033[32m>\033[0m ")

			if len(filename) == 0 {
				Log.Err("wallet name is too short")
				continue
			}

			cmds = append(cmds, filename)
		}

		fmt.Print("Wallet password: ")
		password, err := l.ReadLineWithConfig(lcfg)
		if err != nil {
			Log.Err(err)
		}

		if cmds[0] == "open" {
			Log.Info("opening wallet")

			w, err := wallet.OpenWalletFile(daemon_address, cmds[1]+".keys", []byte(password))
			if err != nil {
				Log.Err(err)
				continue
			}
			return w
		} else {
			fmt.Print("Repeat password: ")
			confirmPass, err := l.ReadLineWithConfig(lcfg)
			if err != nil {
				Log.Err(err)
				continue
			}
			if string(confirmPass) != string(password) {
				Log.Err("password doesn't match")
			}

			if cmd == "create" {
				w, err := wallet.CreateWalletFile(daemon_address, cmds[1]+".keys", []byte(password))
				if err != nil {
					Log.Err("Could not create wallet:", err)
					continue
				}

				return w
			} else if cmd == "restore" {
				Log.Info("restoring wallet")

				l.SetPrompt("Mnemonic seed: ")
				mnemonic, err := l.ReadLine()
				if err != nil {
					Log.Err(err)
					os.Exit(0)
				}

				w, err := wallet.CreateWalletFileFromMnemonic(daemon_address, cmds[1]+".keys",
					mnemonic, []byte(password))
				if err != nil {
					Log.Err(err)
					continue
				}
				return w
			}
		}
	}
}

func main() {
	log_level := flag.Uint("log-level", 1, "sets the log level (range: 0-3)")
	rpc_bind_ip := flag.String("rpc-bind-ip", "127.0.0.1", "starts RPC server on this IP")
	rpc_bind_port := flag.Uint("rpc-bind-port", 0, "starts RPC server on this port")
	rpc_auth := flag.String("rpc-auth", "", "colon-separated username and password, like user:pass")
	open_wallet := flag.String("open-wallet", "", "open a wallet file")
	wallet_password := flag.String("wallet-password", "", "wallet password when using --open-wallet")
	non_interactive := flag.Bool("non-interactive", false, "if set, the node will not process the stdinput. Useful for running as a service.")
	daemon_address := flag.String("daemon-address", default_rpc, "sets the daemon")

	flag.Parse()

	Log.SetLogLevel(uint8(*log_level))

	Log.Info("Starting Virel Wallet CLI")

	var w *wallet.Wallet

	if len(*open_wallet) > 0 {
		wallname := *open_wallet

		if strings.ContainsAny(wallname, "/. \\$") {
			Log.Fatal("invalid wallet name")
		}
		var err error
		w, err = wallet.OpenWalletFile(*daemon_address, wallname+".keys", []byte(*wallet_password))
		if err != nil {
			Log.Fatal(err)
		}
	} else {
		w = initialPrompt(*daemon_address)
	}

	if *rpc_bind_port != 0 {
		if len(*rpc_auth) < 7 {
			Log.Err("rpc-auth is invalid or too short")
		}
		Log.Infof("Starting wallet rpc server on %v:%v", *rpc_bind_ip, *rpc_bind_port)
		startRpcServer(w, *rpc_bind_ip, uint16(*rpc_bind_port), *rpc_auth)
	}

	addr := w.GetAddress()
	if addr.Addr == address.INVALID_ADDRESS {
		Log.Fatal("wallet has invalid address")
	}

	Log.Info("Wallet", w.GetAddress(), "has been loaded")

	Log.Debugf("Address hex: %x", addr.Addr[:])

	err := w.Refresh()
	if err != nil {
		Log.Warn("refresh failed:", err)
	} else {
		Log.Info("Balance:", util.FormatCoin(w.GetBalance()))
		Log.Info("Last nonce:", w.GetLastNonce())
	}

	if !*non_interactive {
		prompts(w)
	} else {
		// wait forever
		c := make(chan bool)
		Log.Err(<-c)
	}
}
