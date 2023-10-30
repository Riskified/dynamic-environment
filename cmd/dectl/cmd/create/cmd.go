package create

import (
	"github.com/riskified/dynamic-environment/cmd/dectl/handler"
	"github.com/spf13/cobra"
)

const (
	deploymentFlag = "deployment"
	namespaceFlag  = "namespace"
)

func NewCmd(ch *handler.CreateHandler) *cobra.Command {
	var createCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a dynamic environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := cmd.Flag(namespaceFlag).Value.String()
			deploymentName := cmd.Flag(deploymentFlag).Value.String()
			ch.Handle(deploymentName, namespace, nil)
			return nil
		},
	}

	createCmd.Flags().StringP(deploymentFlag, "d", "", "Deployment Name")
	createCmd.Flags().StringP(namespaceFlag, "n", "", "Namespace Name")

	_ = createCmd.MarkFlagRequired(deploymentFlag)
	_ = createCmd.MarkFlagRequired(namespaceFlag)

	return createCmd
}
