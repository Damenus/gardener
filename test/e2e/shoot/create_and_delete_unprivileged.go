// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package shoot

import (
	"context"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/gardener/gardener/test/e2e"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
)

var _ = Describe("Shoot Tests", Label("Shoot", "default"), func() {
	f := defaultShootCreationFramework()
	f.GardenerFramework.Config.SkipAccessingShoot = false

	f.Shoot = e2e.DefaultShoot("e2e-unpriv")
	f.Shoot.Spec.Kubernetes.AllowPrivilegedContainers = pointer.Bool(false)
	f.Shoot.Spec.Kubernetes.KubeAPIServer = &gardencorev1beta1.KubeAPIServerConfig{
		AdmissionPlugins: []gardencorev1beta1.AdmissionPlugin{
			{
				Name: "PodSecurity",
				Config: &runtime.RawExtension{
					Raw: []byte(`{
						"apiVersion": "pod-security.admission.config.k8s.io/v1beta1",
						"kind": "PodSecurityConfiguration",
						"defaults": {
						  "enforce": "restricted",
						  "enforce-version": "latest"
						}
					  }`),
				},
			},
		},
	}

	It("Create and Delete Unprivileged Shoot", Label("unprivileged"), func() {
		By("Create Shoot")
		ctx, cancel := context.WithTimeout(parentCtx, 15*time.Minute)
		defer cancel()

		Expect(f.CreateShootAndWaitForCreation(ctx, false)).To(Succeed())
		f.Verify()

		shootClient := f.ShootFramework.ShootClient.Client()

		By("Create pod in the kube-system namespace")
		Expect(shootClient.Create(ctx, newPodForNamespace(metav1.NamespaceSystem))).To(Succeed())

		By("Create pod in the default namespace")
		Expect(shootClient.Create(ctx, newPodForNamespace(metav1.NamespaceDefault))).To(And(
			BeForbiddenError(),
			MatchError(ContainSubstring("pods %q is forbidden: violates PodSecurity %q", "nginx", "restricted:latest")),
		))

		By("Delete Shoot")
		ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
		defer cancel()
		Expect(f.DeleteShootAndWaitForDeletion(ctx, f.Shoot)).To(Succeed())
	})
})

func newPodForNamespace(namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.14.2",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
					},
				},
			},
		},
	}
}
