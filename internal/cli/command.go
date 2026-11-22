package cli

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "gostman",
		Short:         "Send HTTP requests from the command line",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(sendCmd())
	return root
}
