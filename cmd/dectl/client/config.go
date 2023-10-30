package client

import (
	"fmt"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

func GetClientConfig() (*rest.Config, error) {
	kubeconfig, err := getLocalKubeconfig()
	if err != nil {
		return nil, err
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func getLocalKubeconfig() (string, error) {
	if home := homedir.HomeDir(); home != "" {
		return filepath.Join(home, ".kube", "config"), nil
	}
	return "", fmt.Errorf("home directory not found")
}
