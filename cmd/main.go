/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"github.com/riskified/dynamic-environment/internal/controller"
	"github.com/riskified/dynamic-environment/pkg/metrics"
	"github.com/riskified/dynamic-environment/pkg/names"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	// "istio.io/client-go/pkg/apis/networking/v1alpha3"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(riskifiedv1alpha1.AddToScheme(scheme))
	utilruntime.Must(istionetwork.AddToScheme(scheme))

	// if err := v1alpha3.SchemeBuilder.AddToScheme(scheme); err != nil {
	// 	//log.Error(err, "")
	// 	os.Exit(1)
	// }

	//+kubebuilder:scaffold:scheme
}

// A flag parser for list of strings
type arrayFlags []string

func (i *arrayFlags) String() string {
	if i == nil {
		return ""
	}
	return strings.Join(*i, ",")
}

func (i *arrayFlags) Set(value string) error {
	values := strings.Split(value, ",")
	for _, s := range values {
		if s == "" {
			return fmt.Errorf("comma-separated list contains empty value")
		}
	}
	*i = append(*i, values...)
	return nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var versionLabel string
	var defaultVersion string
	var labelsToRemove arrayFlags
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&versionLabel, "version-label", names.DefaultVersionLabel,
		"This is the name of the label used to differentiate versions (default vs overriding).")
	flag.StringVar(&defaultVersion, "default-version", names.DefaultVersion,
		"The global default version - this version is the one that gets the default route. Could be overridden per subset.")
	flag.Var(&labelsToRemove, "remove-labels", "A comma separated list of labels to remove when duplicating deployment.")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "dynamic-environment-lock",
	})

	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.DynamicEnvReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		VersionLabel:   versionLabel,
		DefaultVersion: defaultVersion,
		LabelsToRemove: labelsToRemove,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DynamicEnv")
		os.Exit(1)
	}
	// prevents the webhooks from being started when the ENABLE_WEBHOOKS flag is set to false (for
	// running locally)
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&riskifiedv1alpha1.DynamicEnv{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DynamicEnv")
			os.Exit(1)
		}
	}
	// metrics
	if err = (&metrics.DynamicEnvCollector{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to add metrics collector")
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
