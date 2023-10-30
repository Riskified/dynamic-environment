package client

import (
	"context"
	"fmt"
	"github.com/riskified/dynamic-environment/api/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"os/user"
	"strings"
	"time"
)

const (
	dynamicEnvPersonalLabel = "dynamic-env-personal-label"
	HeaderKey               = "x-dynamic-env"
)

type IDeCTLClient interface {
	CreateDynamicEnv(namespace string, deploymentName string) (*v1alpha1.DynamicEnv, error)
	DeleteDynamicEnv(context context.Context, namespace string, deName string) (rest.Result, error)
	GetDynamicEnvName(deployment, namespace string) string
}

type DeCTLClient struct {
	deClient  IDeClient
	clientSet IKubernetesClientSet
}

// NewDeCTLClient creates a new DeCTLClient instance.
func NewDeCTLClient(deClient IDeClient, clientSet IKubernetesClientSet) (IDeCTLClient, error) {

	return &DeCTLClient{
		deClient:  deClient,
		clientSet: clientSet,
	}, nil
}

// CreateDynamicEnv creates a dynamic environment.
func (c *DeCTLClient) CreateDynamicEnv(namespace string, deploymentName string) (*v1alpha1.DynamicEnv, error) {

	ctx := context.Background()

	deployment, err := c.clientSet.GetDeployment(ctx, namespace, deploymentName)

	if err != nil {
		return &v1alpha1.DynamicEnv{}, err
	}

	de, err := c.deInitialization(deploymentName, namespace, deployment)

	if err != nil {
		return &v1alpha1.DynamicEnv{}, err
	}

	_, err = c.deClient.Post(ctx, namespace, deploymentName, de)

	if err != nil {
		return &v1alpha1.DynamicEnv{}, err
	}

	return c.waitForDynamicEnv(ctx, de.Name, namespace)
}

// DeleteDynamicEnv deletes a dynamic environment.
func (c *DeCTLClient) DeleteDynamicEnv(context context.Context, namespace string, deName string) (rest.Result, error) {
	return c.deClient.Delete(context, namespace, deName)
}

func (c *DeCTLClient) GetDynamicEnvName(deployment, namespace string) string {

	username := "unknown"
	osUser, err := user.Current()

	if err != nil {
		fmt.Println("Warning: failed to Get current user")
	}

	if err == nil && osUser != nil {
		username = strings.ToLower(
			strings.Replace(osUser.Name, " ", "-", -1))
	}

	uniqueName := fmt.Sprintf("de-%s-%s-%s", namespace, deployment, username)

	return uniqueName
}

// getDynamicEnv gets a dynamic environment.
func (c *DeCTLClient) getDynamicEnv(ctx context.Context, deName string, namespace string) (*v1alpha1.DynamicEnv, error) {
	return c.deClient.Get(ctx, deName, namespace)
}

func (c *DeCTLClient) waitForDynamicEnv(ctx context.Context, deName string, namespace string) (*v1alpha1.DynamicEnv, error) {
	errorCh := make(chan error)
	resultCh := make(chan *v1alpha1.DynamicEnv)
	ticker := time.NewTicker(1 * time.Second)

	defer ticker.Stop()

	errorCount := 0       // Initialize an error counter
	degradedCount := 0    // Initialize a degraded counter
	maxErrorCount := 5    // Set the maximum number of errors to 5
	maxDegradedCount := 3 // Set the maximum number of degraded states to 3

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			go func() {
				de, err := c.getDynamicEnv(ctx, deName, namespace)
				if err != nil {
					errorCount++ // Increment the error count
					if errorCount >= maxErrorCount {
						errorCh <- err // If 5 errors occur, return the error
						return
					}
				}

				if de.Status.State == v1alpha1.Degraded {
					degradedCount++ // Increment the degraded count
					if degradedCount >= maxDegradedCount {
						resultCh <- de
					}
				}

				if de.Status.State == v1alpha1.Ready {
					resultCh <- de
					return
				}
			}()

		case err := <-errorCh:
			return nil, err

		case de := <-resultCh:
			return de, nil
		}
	}
}

func (c *DeCTLClient) deInitialization(deploymentName string, namespace string, deployment *v1.Deployment) (v1alpha1.DynamicEnv, error) {

	name := c.GetDynamicEnvName(deploymentName, namespace)
	replicas := int32(1)

	de := v1alpha1.DynamicEnv{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{dynamicEnvPersonalLabel: name},
		},
		Spec: v1alpha1.DynamicEnvSpec{
			IstioMatches: []v1alpha1.IstioMatch{
				{
					Headers: map[string]v1alpha1.StringMatch{
						HeaderKey: {
							Exact: name,
						},
					},
				},
			},
			Subsets: []v1alpha1.Subset{
				{
					Name:      deploymentName,
					Namespace: namespace,
					Replicas:  &replicas,
				},
			},
			Consumers: nil,
		},
	}

	if err := containerInitialization(deployment, &de); err != nil {
		return v1alpha1.DynamicEnv{}, err
	}

	return de, nil
}

func containerInitialization(dep *v1.Deployment, de *v1alpha1.DynamicEnv) error {
	// Check the number of containers defined in the main pod specification
	containerLength := len(dep.Spec.Template.Spec.Containers)
	initContainerLength := len(dep.Spec.Template.Spec.InitContainers)

	// If there is at least one main container, populate the DynamicEnv with its name
	if containerLength > 0 {
		de.Spec.Subsets[0].Containers = []v1alpha1.ContainerOverrides{
			{
				ContainerName: dep.Spec.Template.Spec.Containers[0].Name,
			},
		}
		return nil
	}

	// If there are no main containers but at least one init container, populate the DynamicEnv with the init container's name
	if initContainerLength > 0 {
		de.Spec.Subsets[0].Containers = []v1alpha1.ContainerOverrides{
			{
				ContainerName: dep.Spec.Template.Spec.InitContainers[0].Name,
			},
		}
		return nil
	}

	// If there are no containers defined in the deployment, return an error
	return fmt.Errorf("no containers found in deployment %s", dep.Name)
}
