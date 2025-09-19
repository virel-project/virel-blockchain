package main

import (
	"cmp"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"

	"github.com/virel-project/virel-blockchain/v3/adb"
	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/block"
	"github.com/virel-project/virel-blockchain/v3/blockchain"
	"github.com/virel-project/virel-blockchain/v3/chaintype"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/p2p"
	"github.com/virel-project/virel-blockchain/v3/transaction"
	"github.com/virel-project/virel-blockchain/v3/util"
	"github.com/virel-project/virel-blockchain/v3/util/uint128"

	"github.com/ergochat/readline"
	"github.com/zeebo/blake3"
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

func prompts(bc *blockchain.Blockchain) {
	commands = append(commands, []Cmd{{
		Names: []string{"status", "info"},
		Args:  "",
		Action: func(args []string) {
			Log.Info("Printing blockchain status")

			var stats *blockchain.Stats
			var mem *blockchain.Mempool
			var diff uint128.Uint128
			bc.DB.View(func(tx adb.Txn) error {
				stats = bc.GetStats(tx)
				mem = bc.GetMempool(tx)
				topBlock, err := bc.GetBlock(tx, stats.TopHash)
				if err != nil {
					return err
				}
				diff, err = bc.GetNextDifficulty(tx, topBlock)
				return err
			})

			Log.Infof("Height: %d; Cumulative diff: %.3fk; next diff: %s; hashrate: %s", stats.TopHeight,
				stats.CumulativeDiff.Float64()/1000,
				diff, diff.Div64(config.TARGET_BLOCK_TIME))
			Log.Infof("Sync height: %d", bc.SyncHeight)
			Log.Infof("%d tips (use print_tips for the list of tips)", len(stats.Tips))
			Log.Infof("Mempool: %d entries", len(mem.Entries))

			for i, v := range mem.Entries {
				Log.Infof("%d. %x: fee %d, size %d", i, v.TXID, v.Fee, v.Size)
			}

			err := bc.DB.Update(func(tx adb.Txn) error {
				reorged, err := bc.CheckReorgs(tx, bc.GetStats(tx))
				if reorged {
					Log.Debug("reorganize done")
				}
				return err
			})
			if err != nil {
				Log.Err(err)
			}
		},
	}, {
		Names: []string{"exit", "quit"},
		Args:  "",
		Action: func(args []string) {
			bc.Close()
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
		Names: []string{"connections", "conns", "print_cn", "peers"},
		Args:  "",
		Action: func(args []string) {
			bc.P2P.RLock()
			defer bc.P2P.RUnlock()

			Log.Infof("%s %s %s", util.PadC("Peer ID", 17), util.PadC("IP", 15), "direction")

			cns := make([]*p2p.Connection, len(bc.P2P.Connections))

			i := 0
			for _, conn := range bc.P2P.Connections {
				cns[i] = conn
				i++
			}

			slices.SortStableFunc(cns, func(a, b *p2p.Connection) int {
				return int(a.Time.Sub(b.Time))
			})

			for _, conn := range cns {
				conn.View(func(c *p2p.ConnData) error {
					direction := "inc"
					if c.Outgoing {
						direction = "out"
					}

					Log.Infof("%x… %s %s", c.PeerId[:8], util.PadC(c.IP(), 15), util.PadC(direction, 9))
					return nil
				})
			}
		},
	}, {
		Names: []string{"start_mining"},
		Args:  "<address>",
		Action: func(args []string) {
			if len(args) != 1 {
				Log.Err("Usage: start_mining <address>")
				return
			}

			addr, err := address.FromString(args[0])
			if err != nil {
				Log.Err("Failed to parse address:", args[0])
				return
			}
			Log.Info("Starting miner")
			go bc.StartMining(addr.Addr)
		},
	}, {
		Names: []string{"print_block"},
		Args:  "<height or hash>",
		Action: func(args []string) {
			if len(args) != 1 {
				Log.Err("Usage: print_block <height or hash>")
				return
			}

			err := bc.DB.View(func(tx adb.Txn) error {
				var bl *block.Block
				var inMainchain bool
				if len(args[0]) == 64 {
					bhash, err := hex.DecodeString(args[0])
					if err != nil {
						return err
					}
					hash := [32]byte(bhash)
					bl, err = bc.GetBlock(tx, [32]byte(hash))
					if err != nil {
						return err
					}
					topoHash, err := bc.GetTopo(tx, bl.Height)
					if err != nil {
						Log.Debug(err)
						inMainchain = false
					} else if topoHash == hash {
						inMainchain = true
					} else {
						inMainchain = false
					}
				} else {
					height, err := strconv.ParseUint(args[0], 10, 64)
					if err != nil {
						return err
					}
					bl, err = bc.GetBlockByHeight(tx, height)
					if err != nil {
						return err
					}
					inMainchain = true
				}
				if !inMainchain {
					Log.Info("block is not in mainchain")
				}
				Log.Info("block is in mainchain:", inMainchain)
				Log.Info(bl.String())

				return nil
			})
			if err != nil {
				Log.Err(err)
			}
		},
	}, {
		Names: []string{"print_tx", "print_transaction"},
		Args:  "<hash>",
		Action: func(args []string) {
			if len(args) != 1 || len(args[0]) != 64 {
				Log.Err("Usage: print_tx <hash>")
				return
			}

			txid, err := hex.DecodeString(args[0])
			if err != nil {
				Log.Err(err)
				return
			}

			var tx *transaction.Transaction
			var height uint64
			err = bc.DB.View(func(txn adb.Txn) (err error) {
				stats := bc.GetStats(txn)
				tx, height, err = bc.GetTx(txn, [32]byte(txid), stats.TopHeight)
				return
			})
			if err != nil {
				Log.Err(err)
				return
			}

			if height != 0 {
				Log.Info("transaction found at height", height)
			} else {
				Log.Info("transaction is not included in blockchain")
			}

			Log.Info(tx)
		},
	}, {
		Names: []string{"print_state", "state"},
		Args:  "",
		Action: func(args []string) {
			Log.Info("Printing blockchain state")
			var sum uint64 = 0
			err := bc.DB.View(func(txn adb.Txn) error {
				stats := bc.GetStats(txn)

				err := txn.ForEach(bc.Index.State, func(k, v []byte) error {
					addr := address.Address(k)
					state := &chaintype.State{}

					err := state.Deserialize(v)
					if err != nil {
						return err
					}

					Log.Infof("address: %s balance: %s last nonce: %d", addr, util.FormatCoin(state.Balance),
						state.LastNonce)

					sum += state.Balance

					return nil
				})
				if err != nil {
					return err
				}

				supply := block.GetSupplyAtHeight(stats.TopHeight)
				if sum != supply {
					err = fmt.Errorf("invalid supply %s, expected %s", util.FormatCoin(sum),
						util.FormatCoin(supply))
					Log.Fatal(err)
				}

				return nil
			})
			if err != nil {
				Log.Err(err)
			}

			Log.Info("Done printing blockchain state")
		},
	}, {
		Names: []string{"create_checkpoints"},
		Args:  "[<max height>]",
		Action: func(args []string) {
			err := bc.DB.View(func(tx adb.Txn) error {
				var maxHeight = bc.GetStats(tx).TopHeight
				if len(args) > 0 {
					var err error
					maxHeight, err = strconv.ParseUint(args[0], 10, 64)
					if err != nil {
						Log.Warn("failed to parse height:", err)
					}
				}

				checkpoints, err := bc.CreateCheckpoints(tx, maxHeight, config.DEFAULT_CHECKPOINT_INTERVAL)
				if err != nil {
					return err
				}

				fmt.Printf("// checkpoints generated with: create_checkpoints %d\n", maxHeight)
				fmt.Printf("const CHECKPOINTS_BLAKE3 = \"%x\"\n", blake3.Sum256(checkpoints))
				err = os.WriteFile("checkpoints.bin", checkpoints, 0o666)
				if err != nil {
					return err
				}
				Log.Infof("checkpoints saved to file checkpoints.bin")
				return nil
			})
			if err != nil {
				Log.Err(err)
			}
		},
	}, {
		Names: []string{"print_template", "template", "block_template", "mining_template"},
		Args:  "",
		Action: func(args []string) {
			err := bc.DB.View(func(tx adb.Txn) error {
				bl, min_diff, err := bc.GetBlockTemplate(tx, address.INVALID_ADDRESS)
				if err != nil {
					return err
				}
				Log.Info(bl, "min difficulty:", min_diff)
				return nil
			})
			if err != nil {
				Log.Err(err)
				return
			}
		},
	}, {
		Names: []string{"print_tips", "tips"},
		Args:  "",
		Action: func(args []string) {
			bc.DB.View(func(tx adb.Txn) error {
				stats := bc.GetStats(tx)

				tips := make([]*blockchain.AltchainTip, 0, len(stats.Tips))
				for _, v := range stats.Tips {
					tips = append(tips, v)
				}
				slices.SortFunc(tips, func(a, b *blockchain.AltchainTip) int {
					return cmp.Compare(a.Height, b.Height)
				})

				for _, v := range tips {
					Log.Infof("- %x: Cumulative diff %s; height: %d", v.Hash, v.CumulativeDiff, v.Height)
				}
				return nil
			})
		},
	}, {
		Names: []string{"block_times"},
		Args:  "",
		Action: func(args []string) {
			err := bc.DB.View(func(tx adb.Txn) error {
				stats := bc.GetStats(tx)
				hash := stats.TopHash
				var last float64 = -1

				times := []float64{}
				var sum float64

				const block_time_window = 60 / config.TARGET_BLOCK_TIME * 60 * 4

				for i := 0; i < 4*block_time_window; i++ {
					bl, err := bc.GetBlock(tx, hash)
					if err != nil {
						return err
					}
					if bl.PrevHash() == [32]byte{} {
						break
					}
					if bl.Height < 20 {
						break
					}

					if last != -1 {
						t := math.Round((last-(float64(bl.Timestamp)/1000))*100) / 100
						times = append(times, t)
						sum += t
					}
					last = float64(bl.Timestamp) / 1000
					hash = bl.PrevHash()
				}

				Log.Debug("block times:", times)
				Log.Infof("average block time in the last 4 hours (%d blocks): %.2f", block_time_window, sum/float64(len(times)))

				return nil
			})
			if err != nil {
				Log.Warn(err)
			}
		},
	}, {
		Names: []string{"dag_info"},
		Args:  "",
		Action: func(args []string) {
			err := bc.DB.View(func(txn adb.Txn) error {
				numBlocks, err := txn.Entries(bc.Index.Block)
				if err != nil {
					return err
				}
				st := bc.GetStats(txn)

				var numSides uint64 = 0
				sideDiff := uint128.Zero
				txn.ForEach(bc.Index.Topo, func(k, v []byte) error {
					if len(v) != 32 {
						return fmt.Errorf("invalid value length %d %x", len(v), v)
					}
					bl, err := bc.GetBlock(txn, [32]byte(v))
					if err != nil {
						return err
					}
					numSides += uint64(len(bl.SideBlocks))
					sideDiff = sideDiff.Add(bl.Difficulty.Mul64(2 * uint64(len(bl.SideBlocks))).Div64(3))
					return nil
				})

				orphans := float64(numBlocks) - float64(st.TopHeight) - float64(numSides)

				Log.Info("blocks:", numBlocks, "height:", st.TopHeight, "orphans:", orphans, "sideblocks:", numSides)
				Log.Info("mains:", math.Round(float64(st.TopHeight)/float64(numBlocks)*10000)/100, "%")
				Log.Info("orphans:", math.Round(orphans/float64(numBlocks)*10000)/100, "%")
				Log.Info("sides:", math.Round(float64(numSides)/float64(numBlocks)*10000)/100, "%")
				deltaDiff := st.CumulativeDiff.Sub(sideDiff)
				Log.Info("without side blocks, the cumulative difficulty would be",
					100-(deltaDiff.Float64()/st.CumulativeDiff.Float64()*100), "% lower")
				return nil
			})
			if err != nil {
				Log.Warn(err)
			}
		},
	}, {
		Names: []string{"log_level", "loglevel", "set_log_level"},
		Args:  "<log level>",
		Action: func(args []string) {
			if len(args) < 1 {
				Log.Err("Usage: log_level <log level>")
				return
			}
			num, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil || num > 32 {
				Log.Err("Log level is not valid")
			}
			Log.SetLogLevel(uint8(num))
		},
	}, {
		Names: []string{"print_delegate", "printdelegate", "delegate"},
		Args:  "<delegate id / address>",
		Action: func(args []string) {
			if len(args) < 1 {
				Log.Err("invalid delegate id / address", args)
				return
			}
			args[0] = strings.TrimPrefix(args[0], config.DELEGATE_ADDRESS_PREFIX)
			delid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				Log.Err("invalid delegate address:", err)
				return
			}

			var delegate *chaintype.Delegate
			err = bc.DB.View(func(txn adb.Txn) error {
				delegate, err = bc.GetDelegate(txn, delid)
				return err
			})
			if err != nil {
				Log.Err("failed to get delegate:", err)
				return
			}
			Log.Info(delegate)
		},
	}, {
		Names: []string{"block_queue", "blockqueue"},
		Args:  "",
		Action: func(args []string) {
			bc.BlockQueue.Update(func(qt *blockchain.QueueTx) {
				Log.Info("block queue size:", qt.Length())
				bls := qt.GetBlocks()
				for _, v := range bls {
					Log.Infof("- hash %x expires %d lastreq %d", v.Hash, v.Expires, v.LastRequest)
				}
			})
		},
	}, {
		Names: []string{"orphans"},
		Args:  "",
		Action: func(args []string) {
			err := bc.DB.Update(func(txn adb.Txn) error {
				stats := bc.GetStats(txn)

				type orphaninfo struct {
					Orphan         *blockchain.Orphan
					Height         uint64
					CumulativeDiff uint64
				}

				orphans := make([]orphaninfo, 0, len(stats.Orphans))

				for _, v := range stats.Orphans {
					bl, err := bc.GetBlock(txn, v.Hash)
					if err != nil {
						return err
					}
					orphans = append(orphans, orphaninfo{
						Orphan:         v,
						Height:         bl.Height,
						CumulativeDiff: bl.CumulativeDiff.Lo,
					})
				}

				slices.SortFunc(orphans, func(a, b orphaninfo) int {
					return cmp.Compare(a.Height, b.Height)
				})

				updated := false

				for _, v := range orphans {
					Log.Infof("orphan %d hash: %s, parent: %v", v.Height, v.Orphan.Hash, v.Orphan.PrevHash)
					if stats.Orphans[v.Orphan.PrevHash] == nil {
						prevbl, _ := bc.GetBlock(txn, v.Orphan.PrevHash)
						if prevbl == nil {
							bc.BlockQueue.Update(func(qt *blockchain.QueueTx) {
								qt.SetBlock(blockchain.NewQueuedBlock(v.Orphan.Hash), true)
								Log.Infof("Orphan block %v's parent added to queue", v.Orphan.Hash)
							})
						} else {
							if prevbl.CumulativeDiff.IsZero() {
								Log.Info("could not deorphan block: previous block cumulative difficulty is zero")
							} else {
								Log.Info("deorphaning block", v.Orphan.Hash)
								err := bc.DeorphanBlock(txn, prevbl, v.Orphan.PrevHash, stats)
								if err != nil {
									Log.Err(err)
									return err
								}
								updated = true
							}
						}
					}
				}

				if updated {
					_, err := bc.CheckReorgs(txn, stats)
					if err != nil {
						return err
					}

					return bc.SetStats(txn, stats)
				}
				return nil
			})
			if err != nil {
				Log.Err(err)
			}
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

	l.CaptureExitSignal()

	Log.SetStdout(l.Stdout())
	Log.SetStderr(l.Stderr())

	for {
		line, err := l.ReadLine()
		if err != nil {
			Log.Err(err)
			if len(*cpu_profile) > 0 {
				pprof.StopCPUProfile()
			}
			bc.Close()
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
		}
		if !executed {
			Log.Err("unknown command, use help to see a list of commands")
		}
	}
}
