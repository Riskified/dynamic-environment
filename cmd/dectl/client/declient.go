package client

import (
	"context"
	"fmt"
	"github.com/riskified/dynamic-environment/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
)

const (
	resource = "dynamicenvs"
)

type IDeClient interface {
	Delete(context context.Context, namespace string, deName string) (rest.Result, error)
	Post(ctx context.Context, namespace string, deployment string, de v1alpha1.DynamicEnv) (rest.Result, error)
	Get(ctx context.Context, deName string, namespace string) (*v1alpha1.DynamicEnv, error)
}

type DeClient struct {
	client *rest.RESTClient
}

// NewDeClient creates a new DeClient instance.
func NewDeClient(config *rest.Config) (IDeClient, error) {

	if err := v1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}

	client, err := setRestClient(config)
	if err != nil {
		return nil, err
	}

	return &DeClient{
		client: client,
	}, nil
}

func (c *DeClient) Delete(context context.Context, namespace string, deName string) (rest.Result, error) {
	result := c.client.Delete().
		Namespace(namespace).
		Resource(resource).
		Name(deName).
		Do(context)

	if result.Error() != nil {
		return rest.Result{}, result.Error()
	}

	return result, nil
}

func (c *DeClient) Post(ctx context.Context, namespace string, deployment string, de v1alpha1.DynamicEnv) (rest.Result, error) {

	created := false

	result := c.client.Post().
		Namespace(namespace).
		Resource(resource).
		Name(deployment).Body(&de).
		Do(ctx)

	if result.Error() != nil {
		return rest.Result{}, result.Error()
	}

	result.WasCreated(&created)

	if !created {
		body, _ := result.Raw()
		return rest.Result{}, fmt.Errorf("failed to create dynamic environment:\n %s", string(body))
	}

	return result, nil
}

func (c *DeClient) Get(ctx context.Context, deName string, namespace string) (*v1alpha1.DynamicEnv, error) {
	result := &v1alpha1.DynamicEnv{}
	err := c.client.Get().
		Namespace(namespace).
		Resource(resource).
		Name(deName).
		Do(ctx).Into(result)

	if err != nil {
		return &v1alpha1.DynamicEnv{}, err
	}

	return result, nil
}

func setRestClient(config *rest.Config) (*rest.RESTClient, error) {
	config.ContentConfig.GroupVersion = &v1alpha1.GroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	config.UserAgent = rest.DefaultKubernetesUserAgent()
	client, err := rest.UnversionedRESTClientFor(config)
	if err != nil {
		return nil, err
	}
	return client, nil
}
