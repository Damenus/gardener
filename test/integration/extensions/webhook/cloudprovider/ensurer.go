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

package cloudprovider

import (
	"context"

	"github.com/gardener/gardener/extensions/pkg/webhook/cloudprovider"
	gcontext "github.com/gardener/gardener/extensions/pkg/webhook/context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewEnsurer creates cloudprovider ensurer.
func NewEnsurer(logger logr.Logger) cloudprovider.Ensurer {
	return &ensurer{
		logger: logger,
	}
}

type ensurer struct {
	logger logr.Logger
}

// InjectClient injects the given client into the ensurer.
func (e *ensurer) InjectClient(_ client.Client) error {
	return nil
}

// InjectScheme injects the given scheme into the decoder of the ensurer.
func (e *ensurer) InjectScheme(_ *runtime.Scheme) error {
	return nil
}

// EnsureCloudProviderSecret is implemented on extension side which mutates the cloudprovider secret. contain
// For testing purpose we are mutating the cloudprovider secret's data to check whether this
// function is called in webhook.
func (e *ensurer) EnsureCloudProviderSecret(ctx context.Context, _ gcontext.GardenContext, new, _ *corev1.Secret) error {
	e.logger.Info("Mutate cloudprovider secret", "namespace", new.Namespace, "name", new.Name)
	new.Data["clientID"] = []byte(`foo`)
	new.Data["clientSecret"] = []byte(`bar`)

	return nil
}
