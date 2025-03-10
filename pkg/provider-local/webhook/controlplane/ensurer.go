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

package controlplane

import (
	"context"

	gcontext "github.com/gardener/gardener/extensions/pkg/webhook/context"
	"github.com/gardener/gardener/extensions/pkg/webhook/controlplane/genericmutator"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"

	"github.com/Masterminds/semver"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/go-logr/logr"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	"k8s.io/utils/pointer"
)

// NewEnsurer creates a new controlplane ensurer.
func NewEnsurer(logger logr.Logger) genericmutator.Ensurer {
	return &ensurer{
		logger: logger.WithName("local-controlplane-ensurer"),
	}
}

type ensurer struct {
	genericmutator.NoopEnsurer
	logger logr.Logger
}

func (e *ensurer) EnsureKubeletConfiguration(_ context.Context, _ gcontext.GardenContext, _ *semver.Version, newObj, _ *kubeletconfigv1beta1.KubeletConfiguration) error {
	newObj.FailSwapOn = pointer.Bool(false)
	return nil
}

func (e *ensurer) EnsureETCD(_ context.Context, _ gcontext.GardenContext, newObj, _ *druidv1alpha1.Etcd) error {
	// Remove any affinities from etcd's spec because in the local setup we only have one node,
	// so that pod (anti-) affinities can't apply here.
	newObj.Spec.SchedulingConstraints.Affinity = nil
	return nil
}

// EnsureAdditionalFiles ensures that additional required system files are added.
func (e *ensurer) EnsureAdditionalFiles(_ context.Context, _ gcontext.GardenContext, new, _ *[]extensionsv1alpha1.File) error {
	appendUniqueFile(new, extensionsv1alpha1.File{
		Path:        "/etc/containerd/conf.d/registry-mirror.toml",
		Permissions: pointer.Int32(0644),
		Content: extensionsv1alpha1.FileContent{
			Inline: &extensionsv1alpha1.FileContentInline{
				Encoding: "",
				Data: `[plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5001"]
  endpoint = ["http://gardener-local-control-plane:5001"]
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
  endpoint = ["http://gardener-local-control-plane:5002"]
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."gcr.io"]
  endpoint = ["http://gardener-local-control-plane:5003"]
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."eu.gcr.io"]
  endpoint = ["http://gardener-local-control-plane:5004"]
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."ghcr.io"]
  endpoint = ["http://gardener-local-control-plane:5005"]
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."registry.k8s.io"]
  endpoint = ["http://gardener-local-control-plane:5006"]
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."quay.io"]
  endpoint = ["http://gardener-local-control-plane:5007"]
`,
			},
		},
	})
	return nil
}

// appendUniqueFile appends a unit file only if it does not exist, otherwise overwrite content of previous files
func appendUniqueFile(files *[]extensionsv1alpha1.File, file extensionsv1alpha1.File) {
	resFiles := make([]extensionsv1alpha1.File, 0, len(*files))

	for _, f := range *files {
		if f.Path != file.Path {
			resFiles = append(resFiles, f)
		}
	}

	*files = append(resFiles, file)
}
