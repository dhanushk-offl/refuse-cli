package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/RefuseHQ/refuse-cli/internal/hook"
)

func realHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Manage agent pre-tool-use hooks",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "install <agent>",
		Short: "Install a pre-tool-use hook for an agent (claude-code, codex, ...)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, ok := hook.Get(args[0])
			if !ok {
				return hook.ErrUnknownAgent(args[0])
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if err := a.Install(exe); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "refuse: installed hook for %s\n", a.Name())
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove <agent>",
		Short: "Remove a previously-installed hook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, ok := hook.Get(args[0])
			if !ok {
				return hook.ErrUnknownAgent(args[0])
			}
			if err := a.Remove(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "refuse: removed hook for %s\n", a.Name())
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show installed hooks across known agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, a := range hook.All() {
				st, err := a.Status()
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%-12s  error: %v\n", a.Name(), err)
					continue
				}
				state := "not installed"
				if st.Installed {
					state = "installed"
				} else if !st.Found {
					state = "config not found"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %-18s  %s\n", a.Name(), state, st.ConfigPath)
			}
			return nil
		},
	})

	return cmd
}
