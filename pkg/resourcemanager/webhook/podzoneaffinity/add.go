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

package podzoneaffinity

import (
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	// HandlerName is the name of the webhook handler.
	HandlerName = "pod-zone-affinity"
	// WebhookPath is the path at which the handler should be registered.
	WebhookPath = "/webhooks/pod-zone-affinity"
)

var defaultWebhookConfig WebhookConfig

// WebhookOptions are options for adding the webhook to a Manager.
type WebhookOptions struct {
	enabled bool
}

// WebhookConfig is the completed configuration for the webhook.
type WebhookConfig struct {
	Enabled bool
}

// AddToManagerWithOptions adds the webhook to a Manager with the given config.
func AddToManagerWithOptions(mgr manager.Manager, _ WebhookConfig) error {
	server := mgr.GetWebhookServer()
	server.Register(WebhookPath, &webhook.Admission{
		Handler: NewHandler(mgr.GetLogger().WithName("webhook").WithName(HandlerName)),
	})
	return nil
}

// AddToManager adds the webhook to a Manager using the default config.
func AddToManager(mgr manager.Manager) error {
	return AddToManagerWithOptions(mgr, defaultWebhookConfig)
}

// AddFlags adds the needed command line flags to the given FlagSet.
func (o *WebhookOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.enabled, "pod-zone-affinity-webhook-enabled", false, "enables the pod zone affinity webhook")
}

// Complete completes the given command line flags and set the defaultWebhookConfig accordingly.
func (o *WebhookOptions) Complete() error {
	defaultWebhookConfig = WebhookConfig{
		Enabled: o.enabled,
	}
	return nil
}

// Completed returns the completed WebhookConfig.
func (o *WebhookOptions) Completed() *WebhookConfig {
	return &defaultWebhookConfig
}
