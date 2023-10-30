package client

import (
	"context"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type IKubernetesClientSet interface {
	GetDeployment(ctx context.Context, namespace string, deployment string) (*v1.Deployment, error)
}

type KubernetesClientSet struct {
	client *kubernetes.Clientset
}

func NewClientSet(config *rest.Config) (IKubernetesClientSet, error) {
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &KubernetesClientSet{
		client: clientSet,
	}, nil
}

func (c *KubernetesClientSet) GetDeployment(ctx context.Context, namespace string, deployment string) (*v1.Deployment, error) {
	return c.client.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
}
