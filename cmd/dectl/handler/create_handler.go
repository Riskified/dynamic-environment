package handler

import (
	"context"
	"fmt"
	"github.com/briandowns/spinner"
	"github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/cmd/dectl/client"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Emoji constants
const (
	emojiCheck     = "\U0001f919" // ✌
	emojiFailure   = "\U0001f625" // 😥
	emojiWaveHands = "\U0001f44b" // 👋
	emojiClock     = "\U0001f55b" // 🕛
)

type Callback func(deploymentName string)

type CreateHandler struct {
	client client.IDeCTLClient
}

func NewCreateHandler(client client.IDeCTLClient) *CreateHandler {
	return &CreateHandler{
		client: client,
	}
}

func (h *CreateHandler) Handle(deploymentName, namespace string, callback Callback) {
	h.create(deploymentName, namespace, callback)
}

func (h *CreateHandler) create(deploymentName, namespace string, callback Callback) {
	ctx := context.Background()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	s := spinner.New(spinner.CharSets[39], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Creating dynamic environment %s in namespace %s", deploymentName, namespace)
	s.Start()

	de, err := h.client.CreateDynamicEnv(namespace, deploymentName)

	defer h.cleanup(ctx, namespace, deploymentName)

	s.Stop()

	if err != nil && !errors.IsAlreadyExists(err) {
		fmt.Printf("Error creating dynamic environment: %s\n", err.Error())
		return
	}

	if de.Status.State == v1alpha1.Degraded {
		fmt.Printf("Error: Dynamic environment %s failed to deploy %s\n", de.Name, emojiFailure)
		h.cleanup(ctx, namespace, deploymentName)
		return
	}

	deDeployName := deDeploymentName(de, deploymentName)

	fmt.Printf("Great Success %s\n", emojiCheck)
	fmt.Printf("############################################################################################################\n")
	fmt.Printf("DYNAMIC ENVIRONMENT NAME: %s\n", de.Name)
	fmt.Printf("DYNAMIC ENVIRONMENT HEADER VALUE: %s\n", de.Name)
	fmt.Printf("DYNAMIC ENVIRONMENT HEADER KEY: %s\n", client.HeaderKey)
	fmt.Printf("DYNAMIC ENVIRONMENT DEPLOYMENT: %s\n", deDeployName)
	fmt.Printf("############################################################################################################\n")
	fmt.Printf("You can start using the dynamic environment by adding the following header to your request:\n")
	fmt.Printf("CTRL+C to exit\n")

	if callback != nil {
		callback(deDeployName)
	}

	<-c
	h.cleanup(ctx, namespace, deploymentName)
	os.Exit(0)
}

func deDeploymentName(de *v1alpha1.DynamicEnv, deploymentName string) string {
	return fmt.Sprintf("%s-%s", deploymentName, helpers.UniqueDynamicEnvName(de.Name, de.Namespace))
}

func (h *CreateHandler) cleanup(ctx context.Context, namespace, deploymentName string) {
	fmt.Printf("\nDeleting dynamic environment %s\n", emojiClock)

	deName := h.client.GetDynamicEnvName(deploymentName, namespace)

	if _, err := h.client.DeleteDynamicEnv(ctx, namespace, deName); err != nil {
		fmt.Printf("Error deleting dynamic environment: %s\n", err.Error())
		os.Exit(1)
	}

	fmt.Printf("Deleted successfully %s\n", emojiWaveHands)
}
