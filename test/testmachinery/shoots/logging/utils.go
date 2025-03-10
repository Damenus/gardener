// Copyright 2019 Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	"github.com/gardener/gardener/test/framework"

	"github.com/onsi/ginkgo/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Checks whether required logging resources are present.
// If not, probably the logging feature gate is not enabled.
func hasRequiredResources(ctx context.Context, k8sSeedClient kubernetes.Interface) (bool, error) {
	fluentBit := &appsv1.DaemonSet{}
	if err := k8sSeedClient.Client().Get(ctx, client.ObjectKey{Namespace: garden, Name: fluentBitName}, fluentBit); err != nil {
		return false, err
	}

	loki := &appsv1.StatefulSet{}
	if err := k8sSeedClient.Client().Get(ctx, client.ObjectKey{Namespace: garden, Name: lokiName}, loki); err != nil {
		return false, err
	}

	return true, nil
}

func checkRequiredResources(ctx context.Context, k8sSeedClient kubernetes.Interface) {
	isLoggingEnabled, err := hasRequiredResources(ctx, k8sSeedClient)
	if !isLoggingEnabled {
		message := fmt.Sprintf("Error occurred checking for required logging resources in the seed %s namespace. Ensure that the logging is enabled in GardenletConfiguration: %s", garden, err.Error())
		ginkgo.Fail(message)
	}
}

// WaitUntilLokiReceivesLogs waits until the loki instance in <lokiNamespace> receives <expected> logs for <key>=<value>
func WaitUntilLokiReceivesLogs(ctx context.Context, interval time.Duration, f *framework.ShootFramework, lokiLabels map[string]string, tenant, lokiNamespace, key, value string, expected, delta int, c kubernetes.Interface) error {
	err := retry.Until(ctx, interval, func(ctx context.Context) (done bool, err error) {
		search, err := f.GetLokiLogs(ctx, lokiLabels, tenant, lokiNamespace, key, value, c)
		if err != nil {
			return retry.SevereError(err)
		}
		var actual int
		for _, result := range search.Data.Result {
			currentStr, ok := result.Value[1].(string)
			if !ok {
				return retry.SevereError(fmt.Errorf("Data.Result.Value[1] is not a string for %s=%s", key, value))
			}
			current, err := strconv.Atoi(currentStr)
			if err != nil {
				return retry.SevereError(fmt.Errorf("Data.Result.Value[1] string is not parsable to intiger for %s=%s", key, value))
			}
			actual += current
		}

		log := f.Logger.WithValues("expected", expected, "actual", actual)

		if expected > actual {
			log.Info("Waiting to receive all expected logs")
			return retry.MinorError(fmt.Errorf("received only %d/%d logs", actual, expected))
		} else if expected+delta < actual {
			return retry.SevereError(fmt.Errorf("expected to receive %d logs but was %d", expected, actual))
		}

		log.Info("Received logs", "delta", delta)
		return retry.Ok()
	})

	if err != nil {
		// ctx might have been cancelled already, make sure we still dump logs, so use context.Background()
		dumpLogsCtx, dumpLogsCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer dumpLogsCancel()

		f.Logger.Info("Dump Loki logs")
		if dumpError := f.DumpLogsForPodInNamespace(dumpLogsCtx, c, lokiNamespace, "loki-0"); dumpError != nil {
			f.Logger.Error(dumpError, "Error dumping logs for pod")
		}

		f.Logger.Info("Dump Fluent-bit logs")
		labels := client.MatchingLabels{"app": "fluent-bit"}
		if dumpError := f.DumpLogsForPodsWithLabelsInNamespace(dumpLogsCtx, c, "garden", labels); dumpError != nil {
			f.Logger.Error(dumpError, "Error dumping logs for pod")
		}
	}

	return err
}

func encode(obj runtime.Object) []byte {
	data, _ := json.Marshal(obj)
	return data
}

func create(ctx context.Context, c client.Client, obj client.Object) error {
	obj.SetResourceVersion("")
	return kutil.IgnoreAlreadyExists(c.Create(ctx, obj))
}

func getShootNamesapce(number int) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s%v", simulatesShootNamespacePrefix, number),
		},
	}
}

func getCluster(number int) *extensionsv1alpha1.Cluster {
	shoot := &gardencorev1beta1.Shoot{
		Spec: gardencorev1beta1.ShootSpec{
			Hibernation: &gardencorev1beta1.Hibernation{
				Enabled: pointer.Bool(false),
			},
			Purpose: (*gardencorev1beta1.ShootPurpose)(pointer.String("evaluation")),
		},
	}

	return &extensionsv1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "extensions.gardener.cloud/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s%v", simulatesShootNamespacePrefix, number),
		},
		Spec: extensionsv1alpha1.ClusterSpec{
			Shoot: runtime.RawExtension{
				Raw: encode(shoot),
			},
			CloudProfile: runtime.RawExtension{
				Raw: encode(&gardencorev1beta1.CloudProfile{}),
			},
			Seed: runtime.RawExtension{
				Raw: encode(&gardencorev1beta1.Seed{}),
			},
		},
	}
}

func getLokiShootService(number int) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lokiName,
			Namespace: fmt.Sprintf("%s%v", simulatesShootNamespacePrefix, number),
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceType(corev1.ServiceTypeExternalName),
			ExternalName: "loki-shoots.garden.svc.cluster.local",
		},
	}
}

func getXScopeOrgID(annotations map[string]string) string {
	for key, value := range annotations {
		if key == "nginx.ingress.kubernetes.io/configuration-snippet" {
			configurations := strings.Split(value, ";")
			for _, config := range configurations {
				config = strings.TrimLeft(config, "\t \n")
				if strings.HasPrefix(config, "proxy_set_header") {
					proxySetHeaderFields := strings.Fields(config)
					if len(proxySetHeaderFields) == 3 && proxySetHeaderFields[1] == "X-Scope-OrgID" {
						return proxySetHeaderFields[2]
					}
				}
			}
		}
	}
	return "fake"
}

func getLogCountFromResult(search *framework.SearchResponse) (int, error) {
	var totalLogs int
	for _, result := range search.Data.Result {
		currentStr, ok := result.Value[1].(string)
		if !ok {
			return totalLogs, fmt.Errorf("Data.Result.Value[1] is not a string")
		}
		current, err := strconv.Atoi(currentStr)
		if err != nil {
			return totalLogs, fmt.Errorf("Data.Result.Value[1] string is not parsable to intiger")
		}
		totalLogs += current
	}
	return totalLogs, nil
}

func getConfigMapName(volumes []corev1.Volume, wantedVolumeName string) string {
	for _, volume := range volumes {
		if volume.Name == wantedVolumeName && volume.ConfigMap != nil {
			return volume.ConfigMap.Name
		}
	}
	return ""
}

func newEmptyDirVolume(name, size string) corev1.Volume {
	lokiDataVolumeSize := resource.MustParse(size)
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: &lokiDataVolumeSize,
			},
		},
	}
}

func newPodAntiAffinity(matchLabels map[string]string) *corev1.PodAntiAffinity {
	return &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: matchLabels,
				},
				TopologyKey: "kubernetes.io/hostname",
			},
		},
	}
}

func newGardenNamespace(namespace string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
}
