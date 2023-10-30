package main

import (
	"fmt"
	"github.com/riskified/dynamic-environment/cmd/dectl/client"
	"github.com/riskified/dynamic-environment/cmd/dectl/cmd/create"
	"github.com/riskified/dynamic-environment/cmd/dectl/cmd/root"
	"github.com/riskified/dynamic-environment/cmd/dectl/handler"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"os"
)

func main() {

	cl, err := buildClient()

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ch := handler.NewCreateHandler(cl)

	rootCmd := root.NewCmd()
	rootCmd.AddCommand(
		create.NewCmd(ch),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Error: failed executing command, %v\n", err)
		os.Exit(23)
	}
}

func buildClient() (client.IDeCTLClient, error) {
	config, err := client.GetClientConfig()

	if err != nil {
		return nil, fmt.Errorf("failed initializing config, %w", err)
	}

	decl, err := client.NewDeClient(config)

	if err != nil {
		return nil, fmt.Errorf("failed initializing de client, %w", err)
	}

	clSet, err := client.NewClientSet(config)

	if err != nil {
		return nil, fmt.Errorf("failed initializing client set, %w", err)
	}

	cl, err := client.NewDeCTLClient(decl, clSet)

	if err != nil {
		return nil, fmt.Errorf("failed initializing dectl client, %w", err)
	}

	return cl, nil
}
