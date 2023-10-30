package client_test

import (
	"context"
	"github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/cmd/dectl/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/api/apps/v1"
	v1spec "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"testing"
)

const (
	deName         = "my-de"
	deploymentName = "my-dep"
	containerName  = "my-container"
	namespace      = "my-namespace"
)

// DeClientMock is a mock implementation of the IDeClient interface.
type DeClientMock struct {
	mock.Mock
}

func (m *DeClientMock) Delete(ctx context.Context, namespace string, deName string) (rest.Result, error) {
	args := m.Called(ctx, namespace, deName)
	return args.Get(0).(rest.Result), args.Error(1)
}

func (m *DeClientMock) Post(ctx context.Context, namespace string, deployment string, de v1alpha1.DynamicEnv) (rest.Result, error) {
	args := m.Called(ctx, namespace, deployment, de)
	return args.Get(0).(rest.Result), args.Error(1)
}

func (m *DeClientMock) Get(ctx context.Context, deName string, namespace string) (*v1alpha1.DynamicEnv, error) {
	args := m.Called(ctx, deName, namespace)
	return args.Get(0).(*v1alpha1.DynamicEnv), args.Error(1)
}

// KubernetesClientSetMock is a mock implementation of the IKubernetesClientSet interface.
type KubernetesClientSetMock struct {
	mock.Mock
}

func (m *KubernetesClientSetMock) GetDeployment(ctx context.Context, namespace string, deployment string) (*v1.Deployment, error) {
	args := m.Called(ctx, namespace, deployment)
	return args.Get(0).(*v1.Deployment), args.Error(1)
}

func NewDeClientMock(t mockConstructorTesting) client.IDeClient {
	mock := &DeClientMock{}
	mock.Mock.Test(t)
	t.Cleanup(func() { mock.AssertExpectations(t) })
	return mock
}

func NewKubernetesClientSetMock(t mockConstructorTesting) client.IKubernetesClientSet {
	mock := &KubernetesClientSetMock{}
	mock.Mock.Test(t)
	t.Cleanup(func() { mock.AssertExpectations(t) })
	return mock
}

type mockConstructorTesting interface {
	mock.TestingT
	Cleanup(func())
}

func Test_CreateDynamicEnv(t *testing.T) {

	deClient := NewDeClientMock(t)
	clientSet := NewKubernetesClientSetMock(t)
	deMock := deClient.(*DeClientMock)
	setMock := clientSet.(*KubernetesClientSetMock)

	// Set up expectations for the methods you want to mock.
	mockReturnDeClient(deMock, v1alpha1.Ready)
	// set up expectations for the methods you want to mock.
	mockReturnKubernetesClientSet(setMock)

	// Create an instance of the DeCTLClient.
	deCTLClient, err := client.NewDeCTLClient(deClient, clientSet)

	assert.NoError(t, err)
	deRes, err := deCTLClient.CreateDynamicEnv(namespace, deploymentName)

	assert.NoError(t, err)
	assert.Equal(t, deRes.Name, deName)
	assert.Equal(t, deRes.Namespace, namespace)
	assert.Equal(t, deRes.Status.State, v1alpha1.Ready)
}

func Test_CreateDynamicEnv_Degraded(t *testing.T) {

	deClient := NewDeClientMock(t)
	clientSet := NewKubernetesClientSetMock(t)
	deMock := deClient.(*DeClientMock)
	setMock := clientSet.(*KubernetesClientSetMock)

	// Set up expectations for the methods you want to mock.
	mockReturnDeClient(deMock, v1alpha1.Degraded)
	// set up expectations for the methods you want to mock.
	mockReturnKubernetesClientSet(setMock)

	// Create an instance of the DeCTLClient.
	deCTLClient, err := client.NewDeCTLClient(deClient, clientSet)

	assert.NoError(t, err)
	deRes, err := deCTLClient.CreateDynamicEnv(namespace, deploymentName)

	assert.NoError(t, err)
	assert.Equal(t, deRes.Name, deName)
	assert.Equal(t, deRes.Namespace, namespace)
	assert.Equal(t, deRes.Status.State, v1alpha1.Degraded)

}

func Test_DeleteDynamicEnv(t *testing.T) {
	deClient := NewDeClientMock(t)
	clientSet := NewKubernetesClientSetMock(t)
	deMock := deClient.(*DeClientMock)

	// Set up expectations for the methods you want to mock.
	deMock.On("Delete", mock.Anything, mock.Anything, mock.Anything).Return(rest.Result{}, nil)

	// Create an instance of the DeCTLClient.
	deCTLClient, err := client.NewDeCTLClient(deClient, clientSet)
	assert.NoError(t, err)

	_, err = deCTLClient.DeleteDynamicEnv(context.Background(), namespace, deName)
	assert.NoError(t, err)

	// Verify that the Delete method was called with the correct arguments.
	deMock.AssertCalled(t, "Delete", mock.Anything, namespace, deName)
}

func mockReturnKubernetesClientSet(setMock *KubernetesClientSetMock) {
	setMock.On("GetDeployment", mock.Anything, mock.Anything, mock.Anything).Return(&v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
		Spec: v1.DeploymentSpec{
			Template: v1spec.PodTemplateSpec{
				Spec: v1spec.PodSpec{
					Containers: []v1spec.Container{
						{
							Name: containerName,
						},
					},
				},
			},
		},
	},
		nil)
}

func mockReturnDeClient(deMock *DeClientMock, globalStatus v1alpha1.GlobalReadyStatus) {
	//deMock.On("Delete", mock.Anything, mock.Anything, mock.Anything).Return(rest.Result{}, nil)
	deMock.On("Post", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(rest.Result{}, nil)
	deMock.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(&v1alpha1.DynamicEnv{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deName,
			Namespace: namespace,
		},
		Status: v1alpha1.DynamicEnvStatus{
			State: globalStatus,
		},
	}, nil)
}
