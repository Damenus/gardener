// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package internal

import (
	"context"
	"fmt"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/client/kubernetes/clientmap"
	shootpkg "github.com/gardener/gardener/pkg/operation/shoot"
	"github.com/gardener/gardener/pkg/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	baseconfig "k8s.io/component-base/config"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// shootClientMap is a ClientMap for requesting and storing clients for Shoot clusters.
type shootClientMap struct {
	clientmap.ClientMap
}

// NewShootClientMap creates a new shootClientMap with the given factory.
func NewShootClientMap(log logr.Logger, factory *ShootClientSetFactory) clientmap.ClientMap {
	logger := log.WithValues("clientmap", "ShootClientMap")
	factory.clientKeyToSeedInfo = make(map[ShootClientSetKey]seedInfo)
	factory.log = logger
	return &shootClientMap{
		ClientMap: NewGenericClientMap(factory, logger, clock.RealClock{}),
	}
}

// ShootClientSetFactory is a ClientSetFactory that can produce new ClientSets to Shoot clusters.
type ShootClientSetFactory struct {
	// GetGardenClient is a func that will be used to get a client to the garden cluster to retrieve the Shoot's
	// Project name (which is used for determining the Shoot's technical ID).
	GetGardenClient func(ctx context.Context) (kubernetes.Interface, error)
	// GetSeedClient is a func that will be used to get a client to the Shoot's Seed cluster to retrieve the Shoot's
	// kubeconfig secret ('gardener-internal' or 'gardener').
	GetSeedClient func(ctx context.Context, name string) (kubernetes.Interface, error)
	// ClientConnectionConfiguration is the configuration that will be used by created ClientSets.
	ClientConnectionConfig baseconfig.ClientConnectionConfiguration

	// log is a logger for logging entries related to creating Shoot ClientSets.
	log logr.Logger

	clientKeyToSeedInfo map[ShootClientSetKey]seedInfo
}

type seedInfo struct {
	namespace string
	seedName  string
}

// CalculateClientSetHash calculates a SHA256 hash of the kubeconfig in the 'gardener' secret in the Shoot's Seed namespace.
func (f *ShootClientSetFactory) CalculateClientSetHash(ctx context.Context, k clientmap.ClientSetKey) (string, error) {
	_, hash, err := f.getSecretAndComputeHash(ctx, k)
	if err != nil {
		return "", err
	}

	return hash, nil
}

// NewClientSet creates a new ClientSet for a Shoot cluster.
func (f *ShootClientSetFactory) NewClientSet(ctx context.Context, k clientmap.ClientSetKey) (kubernetes.Interface, string, error) {
	kubeconfigSecret, hash, err := f.getSecretAndComputeHash(ctx, k)
	if err != nil {
		return nil, "", err
	}

	// Kubeconfig secrets are created with empty authinfo and it's expected that gardener-resource-manager eventually
	// populates a token, so let's check whether the read secret already contains authinfo
	tokenPopulated, err := isTokenPopulated(kubeconfigSecret)
	if err != nil {
		return nil, "", err
	}
	if !tokenPopulated {
		return nil, "", fmt.Errorf("token for shoot kubeconfig was not populated yet")
	}

	clientSet, err := NewClientFromSecretObject(kubeconfigSecret,
		kubernetes.WithClientConnectionOptions(f.ClientConnectionConfig),
		kubernetes.WithClientOptions(client.Options{Scheme: kubernetes.ShootScheme}),
		kubernetes.WithDisabledCachedClient(),
	)
	if err != nil {
		return nil, "", err
	}

	return clientSet, hash, nil
}

func (f *ShootClientSetFactory) getSecretAndComputeHash(ctx context.Context, k clientmap.ClientSetKey) (*corev1.Secret, string, error) {
	key, ok := k.(ShootClientSetKey)
	if !ok {
		return nil, "", fmt.Errorf("unsupported ClientSetKey: expected %T got %T", ShootClientSetKey{}, k)
	}

	seedNamespace, seedClient, err := f.getSeedNamespace(ctx, key)
	if err != nil {
		return nil, "", err
	}

	kubeconfigSecret := &corev1.Secret{}
	if err := seedClient.Client().Get(ctx, client.ObjectKey{Namespace: seedNamespace, Name: f.secretName(seedNamespace)}, kubeconfigSecret); err != nil {
		return nil, "", err
	}

	return kubeconfigSecret, utils.ComputeSHA256Hex(kubeconfigSecret.Data[kubernetes.KubeConfig]), nil
}

func (f *ShootClientSetFactory) secretName(seedNamespace string) string {
	secretName := v1beta1constants.SecretNameGardener

	// If the gardenlet runs in the same cluster like the API server of the shoot then use the internal kubeconfig
	// and communicate internally. Otherwise, fall back to the "external" kubeconfig and communicate via the
	// load balancer of the shoot API server.
	addr, err := LookupHost(fmt.Sprintf("%s.%s.svc", v1beta1constants.DeploymentNameKubeAPIServer, seedNamespace))
	if err != nil {
		f.log.Info("Service DNS name lookup of kube-apiserver failed, falling back to external kubeconfig", "error", err)
	} else if len(addr) > 0 {
		secretName = v1beta1constants.SecretNameGardenerInternal
	}

	return secretName
}

var _ clientmap.Invalidate = &ShootClientSetFactory{}

// InvalidateClient invalidates information cached for the given ClientSetKey in the factory.
func (f *ShootClientSetFactory) InvalidateClient(k clientmap.ClientSetKey) error {
	key, ok := k.(ShootClientSetKey)
	if !ok {
		return fmt.Errorf("unsupported ClientSetKey: expected %T got %T", ShootClientSetKey{}, k)
	}
	delete(f.clientKeyToSeedInfo, key)
	return nil
}

func (f *ShootClientSetFactory) seedInfoFromCache(ctx context.Context, key ShootClientSetKey) (string, kubernetes.Interface, error) {
	cache, ok := f.clientKeyToSeedInfo[key]
	if !ok {
		return "", nil, fmt.Errorf("no seed info cached for client %s", key)
	}
	seedClient, err := f.GetSeedClient(ctx, cache.seedName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get seed client from cached seed info %w", err)
	}

	return cache.namespace, seedClient, nil
}

func (f *ShootClientSetFactory) seedInfoToCache(key ShootClientSetKey, namespace, seedName string) {
	f.clientKeyToSeedInfo[key] = seedInfo{
		namespace: namespace,
		seedName:  seedName,
	}
}

func (f *ShootClientSetFactory) getSeedNamespace(ctx context.Context, key ShootClientSetKey) (string, kubernetes.Interface, error) {
	if namespace, seedClient, err := f.seedInfoFromCache(ctx, key); err == nil {
		return namespace, seedClient, nil
	}

	gardenClient, err := f.GetGardenClient(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get garden client: %w", err)
	}

	shoot := &gardencorev1beta1.Shoot{}
	if err := gardenClient.Client().Get(ctx, client.ObjectKey{Namespace: key.Namespace, Name: key.Name}, shoot); err != nil {
		return "", nil, fmt.Errorf("failed to get Shoot object %q: %w", key.Key(), err)
	}

	seedName := shoot.Spec.SeedName
	if seedName == nil {
		return "", nil, fmt.Errorf("shoot %q is not scheduled yet", key.Key())
	}

	seedClient, err := f.GetSeedClient(ctx, *seedName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get seed client: %w", err)
	}

	var namespace string
	if len(shoot.Status.TechnicalID) > 0 {
		namespace = shoot.Status.TechnicalID
	} else {
		project, err := ProjectForNamespaceFromReader(ctx, gardenClient.Client(), shoot.Namespace)
		if err != nil {
			return "", seedClient, fmt.Errorf("failed to get Project for Shoot %q: %w", key.Key(), err)
		}
		namespace = shootpkg.ComputeTechnicalID(project.Name, shoot)
	}

	f.seedInfoToCache(key, namespace, *seedName)

	return namespace, seedClient, nil
}

// ShootClientSetKey is a ClientSetKey for a Shoot cluster.
type ShootClientSetKey struct {
	Namespace, Name string
}

// Key returns the string representation of the ClientSetKey.
func (k ShootClientSetKey) Key() string {
	return k.Namespace + "/" + k.Name
}

func isTokenPopulated(secret *corev1.Secret) (bool, error) {
	kubeconfig := &clientcmdv1.Config{}
	if _, _, err := clientcmdlatest.Codec.Decode(secret.Data[kubernetes.KubeConfig], nil, kubeconfig); err != nil {
		return false, err
	}

	var userName string
	for _, namedContext := range kubeconfig.Contexts {
		if namedContext.Name == kubeconfig.CurrentContext {
			userName = namedContext.Context.AuthInfo
			break
		}
	}

	for _, users := range kubeconfig.AuthInfos {
		if users.Name == userName {
			if len(users.AuthInfo.Token) > 0 {
				return true, nil
			}
			return false, nil
		}
	}

	return false, nil
}
