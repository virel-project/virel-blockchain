package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/virel-project/virel-blockchain/v2/blockchain"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/logger"
	"github.com/virel-project/virel-blockchain/v2/util/updatechecker"
)

var Log = logger.New()

var defaultDataDir string

func init() {
	blockchain.Log = Log

	home, err := os.UserHomeDir()
	if err != nil {
		Log.Fatal(err)
	}

	defaultDataDir = home + "/" + config.NAME + "-" + config.NETWORK_NAME
}

var cpu_profile = flag.String("cpu-profile", "", "write cpu profile to the provided file")

func main() {
	version := flag.Bool("version", false, "prints version and exits")
	p2p_bind_port := flag.Uint("p2p-bind-port", config.P2P_BIND_PORT, "starts P2P server on this port")
	public_rpc := flag.Bool("public-rpc", false, "required for public RPC nodes: blocks private RPC calls and binds on 0.0.0.0")
	rpc_bind_port := flag.Uint("rpc-bind-port", config.RPC_BIND_PORT, "starts RPC server on this port")
	stratum_bind_ip := flag.String("stratum-bind-ip", "127.0.0.1", "use 0.0.0.0 to expose Stratum server")
	stratum_bind_port := flag.Uint("stratum-bind-port", config.STRATUM_BIND_PORT, "")
	log_level := flag.Uint("log-level", 1, "sets the log level")
	private := flag.Bool("private", false, "if set, your ip is not advertised to the network")
	exclusive := flag.Bool("exclusive", false, "if set, the node will not connect to suggested nodes")
	non_interactive := flag.Bool("non-interactive", false, "if set, the node will not process the stdinput. Useful for running as a service.")
	data_dir := flag.String("data-dir", defaultDataDir, "sets the data directory which contains blockchain and peer list")

	var slavechains_stratums *string
	var stratum_wallet *string
	if config.IS_MASTERCHAIN {
		slavechains_stratums = flag.String("slavechains-stratums", "", "comma-separated list of slave chain stratum IP:PORT")
		stratum_wallet = flag.String("mining-wallet", "", "merge mining stratum address")
	}

	flag.Parse()

	go updatechecker.RunUpdateChecker(Log, config.UPDATE_CHECK_URL, config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH)

	if *version {
		fmt.Printf("%s-wallet-cli v%v.%v.%v", config.NAME, config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH)
		os.Exit(0)
	}

	if *cpu_profile != "" {
		f, err := os.Create(*cpu_profile)
		if err != nil {
			Log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			Log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	Log.SetLogLevel(uint8(*log_level))

	Log.Info("Starting", config.NETWORK_NAME, "node")
	Log.Info("Network ID:", config.NETWORK_ID)
	Log.Infof("Version: %d.%d.%d", config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH)
	if config.NETWORK_NAME != "mainnet" {
		Log.Warn("This is a", strings.ToUpper(config.NETWORK_NAME), "node, only for testing the blockchain.")
		Log.Warn("Be aware that any amount transacted in", config.NETWORK_NAME, "is worthless.")
	}

	err := os.MkdirAll(*data_dir, 0o774)
	Log.Debug("failed to create data dir:", err)

	bc := blockchain.New(*data_dir)

	if config.IS_MASTERCHAIN {
		if len(*slavechains_stratums) > 0 {
			stratums := strings.Split(*slavechains_stratums, ",")
			if stratum_wallet == nil || len(*stratum_wallet) == 0 {
				Log.Fatal("you must specify your wallet address when merge mining using --mining-wallet")
			}
			for _, v := range stratums {
				go bc.AddStratum(v, *stratum_wallet, true)
			}
		}
	}

	bind_ip := "127.0.0.1"
	if *public_rpc {
		bind_ip = "0.0.0.0"
	}

	go startRpc(bc, bind_ip, uint16(*rpc_bind_port), *public_rpc)
	go bc.StartStratum(*stratum_bind_ip, uint16(*stratum_bind_port))
	bc.StartP2P(config.SEED_NODES, uint16(*p2p_bind_port), *private, *exclusive)
	go bc.NewStratumJob(true)

	if !*non_interactive {
		prompts(bc)
	} else {
		// wait forever
		c := make(chan bool)
		Log.Err(<-c)
	}
}
