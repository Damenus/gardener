// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package kubelet_test

import (
	"time"

	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/operation/botanist/component/extensions/operatingsystemconfig/original/components"
	"github.com/gardener/gardener/pkg/operation/botanist/component/extensions/operatingsystemconfig/original/components/kubelet"
	"github.com/gardener/gardener/pkg/utils/imagevector"

	"github.com/Masterminds/semver"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("CLIFlags", func() {
	image := &imagevector.Image{
		Name:       "hyperkube",
		Repository: "foo.io/hyperkube",
		Tag:        pointer.String("version"),
	}

	DescribeTable("#CLIFlags",
		func(kubernetesVersion string, criName extensionsv1alpha1.CRIName, image *imagevector.Image, cliFlags components.ConfigurableKubeletCLIFlags, matcher gomegatypes.GomegaMatcher) {
			v := semver.MustParse(kubernetesVersion)
			Expect(kubelet.CLIFlags(v, criName, image, cliFlags)).To(matcher)
		},

		Entry(
			"kubernetes 1.17 w/ docker, w/ imagePullProgressDeadline",
			"1.17.1",
			extensionsv1alpha1.CRINameDocker,
			image,
			components.ConfigurableKubeletCLIFlags{ImagePullProgressDeadline: &metav1.Duration{Duration: 2 * time.Minute}},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--cni-bin-dir=/opt/cni/bin/",
				"--cni-conf-dir=/etc/cni/net.d/",
				"--image-pull-progress-deadline=2m0s",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.17.1",
				"--network-plugin=cni",
				"--volume-plugin-dir=/var/lib/kubelet/volumeplugins",
				"--pod-infra-container-image=foo.io/hyperkube:version",
				"--v=2",
			),
		),
		Entry(
			"kubernetes 1.17 w/ containerd, w/o imagePullProgressDeadline",
			"1.17.1",
			extensionsv1alpha1.CRINameContainerD,
			image,
			components.ConfigurableKubeletCLIFlags{},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.17.1",
				"--volume-plugin-dir=/var/lib/kubelet/volumeplugins",
				"--container-runtime=remote",
				"--container-runtime-endpoint=unix:///run/containerd/containerd.sock",
				"--runtime-cgroups=/system.slice/containerd.service",
				"--v=2",
			),
		),

		Entry(
			"kubernetes 1.18 w/ docker, w/ imagePullProgressDeadline",
			"1.18.1",
			extensionsv1alpha1.CRINameDocker,
			image,
			components.ConfigurableKubeletCLIFlags{ImagePullProgressDeadline: &metav1.Duration{Duration: 2 * time.Minute}},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--cni-bin-dir=/opt/cni/bin/",
				"--cni-conf-dir=/etc/cni/net.d/",
				"--image-pull-progress-deadline=2m0s",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.18.1",
				"--network-plugin=cni",
				"--volume-plugin-dir=/var/lib/kubelet/volumeplugins",
				"--pod-infra-container-image=foo.io/hyperkube:version",
				"--v=2",
			),
		),
		Entry(
			"kubernetes 1.18 w/ containerd, w/o imagePullProgressDeadline",
			"1.18.1",
			extensionsv1alpha1.CRINameContainerD,
			image,
			components.ConfigurableKubeletCLIFlags{},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.18.1",
				"--volume-plugin-dir=/var/lib/kubelet/volumeplugins",
				"--container-runtime=remote",
				"--container-runtime-endpoint=unix:///run/containerd/containerd.sock",
				"--runtime-cgroups=/system.slice/containerd.service",
				"--v=2",
			),
		),

		Entry(
			"kubernetes 1.19 w/ docker, w/ imagePullProgressDeadline",
			"1.19.1",
			extensionsv1alpha1.CRINameDocker,
			image,
			components.ConfigurableKubeletCLIFlags{ImagePullProgressDeadline: &metav1.Duration{Duration: 2 * time.Minute}},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--cni-bin-dir=/opt/cni/bin/",
				"--cni-conf-dir=/etc/cni/net.d/",
				"--image-pull-progress-deadline=2m0s",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.19.1",
				"--network-plugin=cni",
				"--pod-infra-container-image=foo.io/hyperkube:version",
				"--v=2",
			),
		),
		Entry(
			"kubernetes 1.19 w/ containerd, w/o imagePullProgressDeadline",
			"1.19.1",
			extensionsv1alpha1.CRINameContainerD,
			image,
			components.ConfigurableKubeletCLIFlags{},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.19.1",
				"--container-runtime=remote",
				"--container-runtime-endpoint=unix:///run/containerd/containerd.sock",
				"--runtime-cgroups=/system.slice/containerd.service",
				"--v=2",
			),
		),

		Entry(
			"kubernetes 1.20 w/ docker, w/ imagePullProgressDeadline",
			"1.20.1",
			extensionsv1alpha1.CRINameDocker,
			image,
			components.ConfigurableKubeletCLIFlags{ImagePullProgressDeadline: &metav1.Duration{Duration: 2 * time.Minute}},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--cni-bin-dir=/opt/cni/bin/",
				"--cni-conf-dir=/etc/cni/net.d/",
				"--image-pull-progress-deadline=2m0s",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.20.1",
				"--network-plugin=cni",
				"--pod-infra-container-image=foo.io/hyperkube:version",
				"--v=2",
			),
		),
		Entry(
			"kubernetes 1.20 w/ containerd, w/o imagePullProgressDeadline",
			"1.20.1",
			extensionsv1alpha1.CRINameContainerD,
			image,
			components.ConfigurableKubeletCLIFlags{},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.20.1",
				"--container-runtime=remote",
				"--container-runtime-endpoint=unix:///run/containerd/containerd.sock",
				"--runtime-cgroups=/system.slice/containerd.service",
				"--v=2",
			),
		),

		Entry(
			"kubernetes 1.21 w/ docker, w/ imagePullProgressDeadline",
			"1.21.1",
			extensionsv1alpha1.CRINameDocker,
			image,
			components.ConfigurableKubeletCLIFlags{ImagePullProgressDeadline: &metav1.Duration{Duration: 2 * time.Minute}},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--cni-bin-dir=/opt/cni/bin/",
				"--cni-conf-dir=/etc/cni/net.d/",
				"--image-pull-progress-deadline=2m0s",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.21.1",
				"--network-plugin=cni",
				"--pod-infra-container-image=foo.io/hyperkube:version",
				"--v=2",
			),
		),
		Entry(
			"kubernetes 1.21 w/ containerd, w/o imagePullProgressDeadline",
			"1.21.1",
			extensionsv1alpha1.CRINameContainerD,
			image,
			components.ConfigurableKubeletCLIFlags{},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.21.1",
				"--container-runtime=remote",
				"--container-runtime-endpoint=unix:///run/containerd/containerd.sock",
				"--runtime-cgroups=/system.slice/containerd.service",
				"--v=2",
			),
		),

		Entry(
			"kubernetes 1.22 w/ docker, w/ imagePullProgressDeadline",
			"1.22.1",
			extensionsv1alpha1.CRINameDocker,
			image,
			components.ConfigurableKubeletCLIFlags{ImagePullProgressDeadline: &metav1.Duration{Duration: 2 * time.Minute}},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--cni-bin-dir=/opt/cni/bin/",
				"--cni-conf-dir=/etc/cni/net.d/",
				"--image-pull-progress-deadline=2m0s",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.22.1",
				"--network-plugin=cni",
				"--pod-infra-container-image=foo.io/hyperkube:version",
				"--v=2",
			),
		),
		Entry(
			"kubernetes 1.22 w/ containerd, w/o imagePullProgressDeadline",
			"1.22.1",
			extensionsv1alpha1.CRINameContainerD,
			image,
			components.ConfigurableKubeletCLIFlags{},
			ConsistOf(
				"--bootstrap-kubeconfig=/var/lib/kubelet/kubeconfig-bootstrap",
				"--config=/var/lib/kubelet/config/kubelet",
				"--kubeconfig=/var/lib/kubelet/kubeconfig-real",
				"--node-labels=worker.gardener.cloud/kubernetes-version=1.22.1",
				"--container-runtime=remote",
				"--container-runtime-endpoint=unix:///run/containerd/containerd.sock",
				"--runtime-cgroups=/system.slice/containerd.service",
				"--v=2",
			),
		),
	)
})
