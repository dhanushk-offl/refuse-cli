package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/RefuseHQ/refuse-cli/internal/config"
	"github.com/RefuseHQ/refuse-cli/internal/gate"
	"github.com/RefuseHQ/refuse-cli/internal/parsers"
	"github.com/RefuseHQ/refuse-cli/internal/server"
)

// gateCmd is wired up by root.go's newGateCmd stub once we delete that
// stub. For now this lives in the same package; root.go's stub will be
// replaced by this in a follow-up. Exported for that purpose.
func realGateCmd() *cobra.Command {
	var (
		shellCmd string
		agent    string
	)
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Decision engine called by shims and agent hooks (stdin protocol)",
		Long: `Vet a shell command for vulnerable package installs.

When called by an agent hook (Claude Code's PreToolUse, etc.), reads a JSON
payload on stdin describing the tool call. When called by a shim, accepts
the original command via --shell-cmd.

Exit codes:
  0  allow — the install can proceed (or wasn't an install)
  2  block — server returned vulnerable; stderr describes why

Set REFUSE_FAIL_CLOSED=1 to treat server errors as a block (default is
fail-open with a stderr warning).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			command := shellCmd
			if command == "" {
				// Read JSON on stdin (agent hook mode).
				cmdStr, err := gate.ParseHookInput(os.Stdin)
				if err != nil {
					return err
				}
				command = cmdStr
			}
			if command == "" {
				return nil
			}
			subcmds, err := gate.SplitShellCommand(command)
			if err != nil {
				return err
			}
			if len(subcmds) == 0 {
				return nil
			}

			engine := gate.Engine{
				Client:    server.New(cfg.ServerURL, cfg.APIKey),
				Policy:    cfg.Policy,
				AgentMode: agent != "",
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			for _, tokens := range subcmds {
				bin := filepath.Base(tokens[0])
				parser := parsers.ForName(bin)
				if parser == nil {
					continue
				}
				p := parser.Parse(tokens[1:])
				res := engine.Decide(ctx, p)
				if res.Decision == gate.DecisionBlock {
					if res.Message != "" {
						fmt.Fprint(os.Stderr, res.Message)
					}
					os.Exit(2)
				}
				if res.Message != "" {
					fmt.Fprintln(os.Stderr, res.Message)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&shellCmd, "shell-cmd", "", "Vet this command directly instead of reading stdin")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent protocol flavour (claude-code, codex, ...) — reserved for v2")
	return cmd
}
