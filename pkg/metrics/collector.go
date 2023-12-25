/*
Copyright 2023 Riskified Ltd

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

package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	k8smetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	log                     = ctrl.Log.WithName("DynamicEnvCollector")
	runningEnvironmentsDesc = prometheus.NewDesc(
		"dynamic_environments",
		"Number of deployed dynamic environments per namespace",
		[]string{"namespace"},
		nil,
	)
	degradedEnvironmentsDesc = prometheus.NewDesc(
		"degraded_dynamic_environments",
		"Number of dynamic environments in degraded state per namespace",
		[]string{"namespace", "name"},
		nil,
	)
)

type DynamicEnvCollector struct {
	client.Client
}

func (c *DynamicEnvCollector) Describe(descs chan<- *prometheus.Desc) {
	log.Info("Called Describe ...")
	descs <- runningEnvironmentsDesc
	descs <- degradedEnvironmentsDesc
}

func (c *DynamicEnvCollector) Collect(metrics chan<- prometheus.Metric) {
	log.Info("called Collect ...")
	desPerNamespace := make(map[string]float64)
	ctx := context.Background()
	des := &riskifiedv1alpha1.DynamicEnvList{}
	if err := c.List(ctx, des, client.InNamespace("")); err != nil {
		log.Error(err, "Error listing dynamic environments")
		return
	}

	for _, de := range des.Items {
		desPerNamespace[de.Namespace] = desPerNamespace[de.Namespace] + 1
		if de.Status.State == riskifiedv1alpha1.Degraded {
			metrics <- prometheus.MustNewConstMetric(degradedEnvironmentsDesc, prometheus.GaugeValue, 1, de.Namespace, de.Name)
		}
	}
	for ns, val := range desPerNamespace {
		metrics <- prometheus.MustNewConstMetric(runningEnvironmentsDesc, prometheus.GaugeValue, val, ns)
	}
}

func (c *DynamicEnvCollector) Start(ctx context.Context) error {
	log.Info("Starting collector")
	if err := k8smetrics.Registry.Register(c); err != nil {
		return err
	}

	// Block until the context is done.
	<-ctx.Done()
	k8smetrics.Registry.Unregister(c)

	log.Info("Ending collector")
	return nil
}

func (c *DynamicEnvCollector) SetupWithManager(mgr manager.Manager) error {
	return mgr.Add(c)
}
