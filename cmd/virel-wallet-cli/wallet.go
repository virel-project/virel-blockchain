package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/logger"
	"github.com/virel-project/virel-blockchain/v3/util"
	"github.com/virel-project/virel-blockchain/v3/util/updatechecker"
	"github.com/virel-project/virel-blockchain/v3/wallet"

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

	Log.SetStdout(l.Stdout())
	Log.SetStderr(l.Stderr())

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

			w, err := wallet.OpenWalletFile(daemon_address, cmds[1]+".keys", password)
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
				continue
			}

			if cmd == "create" {
				w, err := wallet.CreateWalletFile(daemon_address, cmds[1]+".keys", password)
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
					mnemonic, password)
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
	version := flag.Bool("version", false, "prints version and exits")
	log_level := flag.Uint("log-level", 1, "sets the log level (range: 0-3)")
	rpc_bind_ip := flag.String("rpc-bind-ip", "127.0.0.1", "starts RPC server on this IP")
	rpc_bind_port := flag.Uint("rpc-bind-port", 0, "starts RPC server on this port")
	rpc_auth := flag.String("rpc-auth", "", "colon-separated username and password, like user:pass")
	open_wallet := flag.String("open-wallet", "", "open a wallet file")
	wallet_password := flag.String("wallet-password", "", "wallet password when using --open-wallet")
	non_interactive := flag.Bool("non-interactive", false, "if set, the node will not process the stdinput. Useful for running as a service.")
	daemon_address := flag.String("daemon-address", default_rpc, "sets the daemon")
	start_staking := flag.String("start-staking", "", "starts staking to the provided delegate id")

	flag.Parse()

	if *version {
		fmt.Printf("%s-wallet-cli v%v.%v.%v", config.NAME, config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH)
		os.Exit(0)
	}

	Log.SetLogLevel(uint8(*log_level))

	Log.Infof("Starting Virel Wallet CLI v%v.%v.%v", config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH)

	go updatechecker.RunUpdateChecker(Log, config.UPDATE_CHECK_URL, config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH)

	var w *wallet.Wallet

	if len(*open_wallet) > 0 {
		wallname := *open_wallet

		if strings.ContainsAny(wallname, "/. \\$") {
			Log.Fatal("invalid wallet name")
		}
		var err error
		w, err = wallet.OpenWalletFile(*daemon_address, wallname+".keys", *wallet_password)
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

	Log.Info("Wallet", addr, "has been loaded")
	Log.Infof("Public key: %x", w.GetPubKey())

	Log.Debugf("Address hex: %x", addr.Addr[:])

	if len(*start_staking) > 0 {
		delegateAddr, err := address.FromString(*start_staking)
		if err != nil {
			Log.Err("invalid delegate address:", err)
			return
		}

		go staker(w, delegateAddr.Addr)
	}

	err := w.Refresh()
	if err != nil {
		Log.Warn("refresh failed:", err)
	} else {
		Log.Info("Balance:", util.FormatCoin(w.GetBalance()))
		Log.Infof("Last nonce: %d", w.GetLastNonce())
		if w.GetDelegateId() != 0 {
			Log.Infof("Delegate: %s (%s)", w.GetDelegateName(), address.NewDelegateAddress(w.GetDelegateId()))
			Log.Infof("Staked balance: %s", util.FormatCoin(w.GetStakedBalance()))
		}
	}

	if !*non_interactive {
		prompts(w)
	} else {
		// wait forever
		c := make(chan bool)
		Log.Err(<-c)
	}
}
