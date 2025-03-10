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

package certificate

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/client/kubernetes/clientmap"
	"github.com/gardener/gardener/pkg/client/kubernetes/clientmap/keys"
	"github.com/gardener/gardener/pkg/gardenlet/apis/config"
	bootstraputil "github.com/gardener/gardener/pkg/gardenlet/bootstrap/util"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
)

var (
	// certificateWaitTimeout controls the amount of time we wait for the certificate
	// approval in one iteration.
	certificateWaitTimeout = 15 * time.Minute

	// EventGardenletCertificateRotationFailed is an event reason to describe a failed Gardenlet certificate rotation.
	EventGardenletCertificateRotationFailed = "GardenletCertificateRotationFailed"
)

// Manager can be used to schedule the certificate rotation for the Gardenlet's Garden cluster client certificate
type Manager struct {
	log                    logr.Logger
	clientMap              clientmap.ClientMap
	seedClient             client.Client
	gardenClientConnection *config.GardenClientConnection
	seedName               string
}

// NewCertificateManager creates a certificate manager that can be used to rotate gardenlet's client certificate for the Garden cluster
func NewCertificateManager(log logr.Logger, clientMap clientmap.ClientMap, seedClient client.Client, config *config.GardenletConfiguration) *Manager {
	seedName := bootstraputil.GetSeedName(config.SeedConfig)

	return &Manager{
		log:                    log.WithName("certificate-manager").WithValues("seedName", seedName),
		clientMap:              clientMap,
		seedClient:             seedClient,
		gardenClientConnection: config.GardenClientConnection,
		seedName:               seedName,
	}
}

// ScheduleCertificateRotation waits until the currently used Garden cluster client certificate approaches expiration.
// Then requests a new certificate and stores the kubeconfig in a secret (`gardenClientConnection.kubeconfigSecret`) on the Seed.
// the argument is a context.Cancel function to cancel the context of the Gardenlet used for graceful termination after a successful certificate rotation.
// When the new gardenlet pod is started, it uses the rotated certificate stored in the secret in the Seed cluster
func (cr *Manager) ScheduleCertificateRotation(ctx context.Context, gardenletCancel context.CancelFunc, recorder record.EventRecorder) {
	go wait.Until(func() {
		certificateSubject, dnsSANs, ipSANs, certificateExpirationTime, err := waitForCertificateRotation(ctx, cr.log, cr.seedClient, cr.gardenClientConnection, time.Now)
		if err != nil {
			cr.log.Error(err, "Waiting for the certificate rotation failed")
			return
		}

		err = retry.Until(ctx, certificateWaitTimeout, func(ctx context.Context) (bool, error) {
			ctxWithTimeout, cancel := context.WithTimeout(ctx, certificateWaitTimeout)
			defer cancel()

			err := rotateCertificate(ctxWithTimeout, cr.log, cr.clientMap, cr.seedClient, cr.gardenClientConnection, certificateSubject, dnsSANs, ipSANs)
			if err != nil {
				cr.log.Error(err, "Certificate rotation failed")
				return retry.MinorError(err)
			}
			return retry.Ok()
		})
		if err != nil {
			cr.log.Error(err, "Failed to rotate the kubeconfig for the Garden API Server", "certificateExpirationTime", certificateExpirationTime)
			seed, err := cr.getTargetedSeed(ctx)
			if err != nil {
				cr.log.Error(err, "Failed to record event on Seed announcing the failed certificate rotation")
				return
			}
			recorder.Event(seed, corev1.EventTypeWarning, EventGardenletCertificateRotationFailed, fmt.Sprintf("Failed to rotate the kubeconfig for the Garden API Server. Certificate expires in %s (%s): %v", certificateExpirationTime.UTC().Sub(time.Now().UTC()).Round(time.Second).String(), certificateExpirationTime.Round(time.Second).String(), err))
			return
		}

		cr.log.Info("Terminating Gardenlet after successful certificate rotation")
		gardenletCancel()
	}, time.Second, ctx.Done())
}

// getTargetedSeed returns the Seed that this Gardenlet is reconciling
func (cr *Manager) getTargetedSeed(ctx context.Context) (*gardencorev1beta1.Seed, error) {
	gardenClient, err := cr.clientMap.GetClient(ctx, keys.ForGarden())
	if err != nil {
		return nil, err
	}

	seed := &gardencorev1beta1.Seed{}
	if err := gardenClient.Client().Get(ctx, client.ObjectKey{Name: cr.seedName}, seed); err != nil {
		return nil, err
	}

	return seed, nil
}

// waitForCertificateRotation determines and waits for the certificate rotation deadline.
// Reschedules the certificate rotation in case the underlying certificate expiration date has changed in the meanwhile.
func waitForCertificateRotation(
	ctx context.Context,
	log logr.Logger,
	seedClient client.Client,
	gardenClientConnection *config.GardenClientConnection,
	now func() time.Time,
) (
	*pkix.Name,
	[]string,
	[]net.IP,
	*time.Time,
	error,
) {
	kubeconfigSecret, cert, err := readCertificateFromKubeconfigSecret(ctx, log, seedClient, gardenClientConnection)
	if err != nil {
		return nil, []string{}, []net.IP{}, nil, err
	}

	if kubeconfigSecret.Annotations[v1beta1constants.GardenerOperation] == "renew" {
		log.Info("Certificate expiration has not passed but immediate renewal was requested", "notAfter", cert.Leaf.NotAfter)
		return &cert.Leaf.Subject, cert.Leaf.DNSNames, cert.Leaf.IPAddresses, &cert.Leaf.NotAfter, nil
	}

	deadline := nextRotationDeadline(*cert, gardenClientConnection.KubeconfigValidity)
	log.Info("Determined certificate expiration and rotation deadline", "notAfter", cert.Leaf.NotAfter, "rotationDeadline", deadline)

	if sleepInterval := deadline.Sub(now()); sleepInterval > 0 {
		log.Info("Waiting for next certificate rotation", "interval", sleepInterval)
		// block until certificate rotation or until context is cancelled
		select {
		case <-ctx.Done():
			return nil, []string{}, []net.IP{}, nil, ctx.Err()
		case <-time.After(sleepInterval):
		}
	}

	log.Info("Starting the certificate rotation")

	// check the validity of the certificate again. It might have changed
	_, currentCert, err := readCertificateFromKubeconfigSecret(ctx, log, seedClient, gardenClientConnection)
	if err != nil {
		return nil, []string{}, []net.IP{}, nil, err
	}

	if currentCert.Leaf.NotAfter != cert.Leaf.NotAfter {
		return nil, []string{}, []net.IP{}, nil, fmt.Errorf("the certificates expiration date has been changed. Rescheduling certificate rotation")
	}

	return &currentCert.Leaf.Subject, currentCert.Leaf.DNSNames, currentCert.Leaf.IPAddresses, &currentCert.Leaf.NotAfter, nil
}

func readCertificateFromKubeconfigSecret(ctx context.Context, log logr.Logger, seedClient client.Client, gardenClientConnection *config.GardenClientConnection) (*corev1.Secret, *tls.Certificate, error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := seedClient.Get(ctx, kutil.Key(gardenClientConnection.KubeconfigSecret.Namespace, gardenClientConnection.KubeconfigSecret.Name), kubeconfigSecret); client.IgnoreNotFound(err) != nil {
		return nil, nil, err
	}

	cert, err := GetCurrentCertificate(log, kubeconfigSecret.Data[kubernetes.KubeConfig], gardenClientConnection)
	if err != nil {
		return nil, nil, err
	}

	return kubeconfigSecret, cert, nil
}

// GetCurrentCertificate returns the client certificate which is currently used to communicate with the garden cluster.
func GetCurrentCertificate(log logr.Logger, gardenKubeconfig []byte, gardenClientConnection *config.GardenClientConnection) (*tls.Certificate, error) {
	kubeconfigKey := kutil.ObjectKeyFromSecretRef(*gardenClientConnection.KubeconfigSecret)
	log = log.WithValues("kubeconfigSecret", kubeconfigKey)

	if len(gardenKubeconfig) == 0 {
		log.Info("Kubeconfig secret on the target cluster does not contain a kubeconfig. Falling back to `gardenClientConnection.Kubeconfig`. The secret's `.data` field should contain a key `kubeconfig` that is mapped to a byte representation of the garden kubeconfig")
		// check if there is a locally provided kubeconfig via Gardenlet configuration `gardenClientConnection.Kubeconfig`
		if len(gardenClientConnection.Kubeconfig) == 0 {
			return nil, fmt.Errorf("the kubeconfig secret %q on the target cluster does not contain a kubeconfig and there is no fallback kubeconfig specified in `gardenClientConnection.Kubeconfig`. The secret's `.data` field should contain a key `kubeconfig` that is mapped to a byte representation of the garden kubeconfig. Possibly there was an external change to the secret specified in `gardenClientConnection.KubeconfigSecret`. If this error continues, stop the gardenlet, and either configure it with a fallback kubeconfig in `gardenClientConnection.Kubeconfig`, or provide `gardenClientConnection.KubeconfigBootstrap` to bootstrap a new certificate", kubeconfigKey.String())
		}
	}

	// get a rest config from either the `gardenClientConnection.KubeconfigSecret` or from the fallback kubeconfig specified in `gardenClientConnection.Kubeconfig`
	restConfig, err := kubernetes.RESTConfigFromClientConnectionConfiguration(&gardenClientConnection.ClientConnectionConfiguration, gardenKubeconfig)
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair(restConfig.CertData, restConfig.KeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse X509 certificate from kubeconfig in secret %q on the target cluster: %w", kubeconfigKey.String(), err)
	}

	if len(cert.Certificate) < 1 {
		return nil, fmt.Errorf("the X509 certificate from kubeconfig in secret %q on the target cluster is invalid. No cert/key data found", kubeconfigKey.String())
	}

	certs, err := x509.ParseCertificates(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("the X509 certificate from kubeconfig in secret %q on the target cluster cannot be parsed: %w", kubeconfigKey.String(), err)
	}

	if len(certs) < 1 {
		return nil, fmt.Errorf("the X509 certificate from kubeconfig in secret %q on the target cluster is invalid", kubeconfigKey.String())
	}

	cert.Leaf = certs[0]
	return &cert, nil
}

// rotateCertificate uses an already existing garden client (already bootstrapped) to request a new client certificate
// after successful retrieval of the client certificate, updates the secret in the seed with the rotated kubeconfig
func rotateCertificate(
	ctx context.Context,
	log logr.Logger,
	clientMap clientmap.ClientMap,
	seedClient client.Client,
	gardenClientConnection *config.GardenClientConnection,
	certificateSubject *pkix.Name,
	dnsSANs []string,
	ipSANs []net.IP,
) error {
	// client to communicate with the Garden API server to create the CSR
	gardenClient, err := clientMap.GetClient(ctx, keys.ForGarden())
	if err != nil {
		return err
	}

	// request new client certificate
	certData, privateKeyData, _, err := RequestCertificate(ctx, log, gardenClient.Kubernetes(), certificateSubject, dnsSANs, ipSANs, gardenClientConnection.KubeconfigValidity.Validity)
	if err != nil {
		return err
	}

	kubeconfigKey := kutil.ObjectKeyFromSecretRef(*gardenClientConnection.KubeconfigSecret)
	log = log.WithValues("kubeconfigSecret", kubeconfigKey)
	log.Info("Updating kubeconfig secret in target cluster with rotated certificate")

	_, err = bootstraputil.UpdateGardenKubeconfigSecret(ctx, gardenClient.RESTConfig(), certData, privateKeyData, seedClient, kubeconfigKey)
	if err != nil {
		return fmt.Errorf("unable to update kubeconfig secret %q on the target cluster during certificate rotation: %w", kubeconfigKey.String(), err)
	}

	return nil
}
