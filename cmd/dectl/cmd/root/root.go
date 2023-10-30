package root

import (
	"fmt"
	"github.com/spf13/cobra"
)

// NewCmd creates instance of root "dectl" Cobra Command with flags and execution logic defined.
func NewCmd() *cobra.Command {

	rootCmd := &cobra.Command{
		SilenceErrors: true,
		Use:           "dectl",
		Short:         `dectl lets you create your dynamic environment easily!`,

		RunE: func(cmd *cobra.Command, args []string) error {
			printCover()
			err := cmd.Help()
			if err != nil {
				return fmt.Errorf("failed to print help, %w", err)
			}
			return nil
		},
	}

	return rootCmd
}

func printCover() {
	fmt.Println("  ____                              _      _____            _                                      _   ")
	fmt.Println(" |  _ \\ _   _ _ __   __ _ _ __ ___ (_) ___| ____|_ ____   _(_)_ __ ___  _ __  _ __ ___   ___ _ __ | |_ ")
	fmt.Println(" | | | | | | | '_ \\ / _` | '_ ` _ \\| |/ __|  _| | '_ \\ \\ / / | '__/ _ | '_ \\| '_ ` _ \\ / _ \\ '_ \\ | __|")
	fmt.Println(" | |_| | |_| | | | | (_| | | | | | | | (__| |___| | | \\ V /| | | | (_) | | | | | | | | |  __/ | | | |_ ")
	fmt.Println(" |____/ \\__, |_| |_|\\__,_|_| |_| |_|_|\\___|_____|_| |_|\\_/ |_|_|  \\___/|_| |_|_| |_| |_|\\___|_| |_|\\__|")
	fmt.Println("        |___/                                                                                          ")
	fmt.Println("                                                                                                   ")
}
