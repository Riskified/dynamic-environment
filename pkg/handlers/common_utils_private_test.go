package handlers

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A hack for implementing a k8s client inside a test function.
// see: https://stackoverflow.com/questions/31362044/anonymous-interface-implementation-in-golang
type MockClient struct {
	client.Client
	getMethod  func(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error
	listMethod func(context.Context, client.ObjectList, ...client.ListOption) error
}

func (m MockClient) Get(c context.Context, ns types.NamespacedName, o client.Object, _ ...client.GetOption) error {
	return m.getMethod(c, ns, o)
}

func (m MockClient) List(c context.Context, l client.ObjectList, o ...client.ListOption) error {
	return m.listMethod(c, l, o...)
}
