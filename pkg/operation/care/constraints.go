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

package care

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	gardencorev1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/operation"
	"github.com/gardener/gardener/pkg/operation/botanist/component/resourcemanager"
	"github.com/gardener/gardener/pkg/operation/botanist/matchers"
	"github.com/gardener/gardener/pkg/operation/shoot"
	"github.com/gardener/gardener/pkg/utils"
	secretutils "github.com/gardener/gardener/pkg/utils/secrets"
	secretsmanager "github.com/gardener/gardener/pkg/utils/secrets/manager"

	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WebhookMaximumTimeoutSecondsNotProblematic is the maximum timeout in seconds a webhooks on critical resources can
// have in order to not be considered as a problematic webhook by the constraints checks. Any webhook on critical
// resources with a larger timeout is considered to be problematic.
const WebhookMaximumTimeoutSecondsNotProblematic = 15

func shootHibernatedConstraints(conditions ...gardencorev1beta1.Condition) []gardencorev1beta1.Condition {
	hibernationConditions := make([]gardencorev1beta1.Condition, 0, len(conditions))
	for _, cond := range conditions {
		hibernationConditions = append(hibernationConditions, gardencorev1beta1helper.UpdatedCondition(cond, gardencorev1beta1.ConditionTrue, "ConstraintNotChecked", "Shoot cluster has been hibernated."))
	}
	return hibernationConditions
}

func shootControlPlaneNotRunningConstraints(conditions ...gardencorev1beta1.Condition) []gardencorev1beta1.Condition {
	constraints := make([]gardencorev1beta1.Condition, 0, len(conditions))
	for _, cond := range conditions {
		constraints = append(constraints, gardencorev1beta1helper.UpdatedCondition(cond, gardencorev1beta1.ConditionFalse, "ConstraintNotChecked", "Shoot control plane is not running at the moment."))
	}
	return constraints
}

// Constraint contains required information for shoot constraint checks.
type Constraint struct {
	shoot *shoot.Shoot

	seedClient             client.Client
	initializeShootClients ShootClientInit
	shootClient            client.Client

	log   logr.Logger
	clock clock.Clock
}

// NewConstraint returns a new constraint instance.
func NewConstraint(clock clock.Clock, op *operation.Operation, shootClientInit ShootClientInit) *Constraint {
	return &Constraint{
		clock:                  clock,
		shoot:                  op.Shoot,
		seedClient:             op.K8sSeedClient.Client(),
		initializeShootClients: shootClientInit,
		log:                    op.Logger,
	}
}

// Check checks all given constraints.
func (c *Constraint) Check(
	ctx context.Context,
	constraints []gardencorev1beta1.Condition,
) []gardencorev1beta1.Condition {
	updatedConstraints := c.constraintsChecks(ctx, constraints)
	lastOp := c.shoot.GetInfo().Status.LastOperation
	lastErrors := c.shoot.GetInfo().Status.LastErrors
	return PardonConditions(updatedConstraints, lastOp, lastErrors)
}

func (c *Constraint) constraintsChecks(
	ctx context.Context,
	constraints []gardencorev1beta1.Condition,
) []gardencorev1beta1.Condition {
	if c.shoot.HibernationEnabled || c.shoot.GetInfo().Status.IsHibernated {
		return shootHibernatedConstraints(constraints...)
	}

	var (
		// required constraints (always present in .status.constraints)
		hibernationPossibleConstraint, maintenancePreconditionsSatisfiedConstraint gardencorev1beta1.Condition
		// optional constraints (not always present in .status.constraints)
		caCertificateValiditiesAcceptableConstraint = gardencorev1beta1.Condition{Type: gardencorev1beta1.ShootCACertificateValiditiesAcceptable}
	)

	for _, cons := range constraints {
		switch cons.Type {
		case gardencorev1beta1.ShootHibernationPossible:
			hibernationPossibleConstraint = cons
		case gardencorev1beta1.ShootMaintenancePreconditionsSatisfied:
			maintenancePreconditionsSatisfiedConstraint = cons
		case gardencorev1beta1.ShootCACertificateValiditiesAcceptable:
			caCertificateValiditiesAcceptableConstraint = cons
		}
	}

	// Check constraints not depending on the shoot's kube-apiserver to be up and running
	status, reason, message, errorCodes, err := c.CheckIfCACertificateValiditiesAcceptable(ctx)
	if err != nil {
		caCertificateValiditiesAcceptableConstraint = gardencorev1beta1helper.UpdatedConditionUnknownError(caCertificateValiditiesAcceptableConstraint, err)
	} else {
		caCertificateValiditiesAcceptableConstraint = gardencorev1beta1helper.UpdatedCondition(caCertificateValiditiesAcceptableConstraint, status, reason, message, errorCodes...)
	}

	// Now check constraints depending on the shoot's kube-apiserver to be up and running
	shootClient, apiServerRunning, err := c.initializeShootClients()
	if err != nil {
		c.log.Error(err, "Could not initialize Shoot client for constraints check")

		message := fmt.Sprintf("Could not initialize Shoot client for constraints check: %+v", err)
		hibernationPossibleConstraint = gardencorev1beta1helper.UpdatedConditionUnknownErrorMessage(hibernationPossibleConstraint, message)
		maintenancePreconditionsSatisfiedConstraint = gardencorev1beta1helper.UpdatedConditionUnknownErrorMessage(maintenancePreconditionsSatisfiedConstraint, message)

		return filterOptionalConstraints(
			[]gardencorev1beta1.Condition{hibernationPossibleConstraint, maintenancePreconditionsSatisfiedConstraint},
			[]gardencorev1beta1.Condition{caCertificateValiditiesAcceptableConstraint},
		)
	}
	if !apiServerRunning {
		// don't check constraints if API server has already been deleted or has not been created yet
		return filterOptionalConstraints(
			shootControlPlaneNotRunningConstraints(hibernationPossibleConstraint, maintenancePreconditionsSatisfiedConstraint),
			[]gardencorev1beta1.Condition{caCertificateValiditiesAcceptableConstraint},
		)
	}
	c.shootClient = shootClient.Client()

	status, reason, message, errorCodes, err = c.CheckForProblematicWebhooks(ctx)
	if err != nil {
		hibernationPossibleConstraint = gardencorev1beta1helper.UpdatedConditionUnknownError(hibernationPossibleConstraint, err)
		maintenancePreconditionsSatisfiedConstraint = gardencorev1beta1helper.UpdatedConditionUnknownError(maintenancePreconditionsSatisfiedConstraint, err)
	} else {
		hibernationPossibleConstraint = gardencorev1beta1helper.UpdatedCondition(hibernationPossibleConstraint, status, reason, message, errorCodes...)
		maintenancePreconditionsSatisfiedConstraint = gardencorev1beta1helper.UpdatedCondition(maintenancePreconditionsSatisfiedConstraint, status, reason, message, errorCodes...)
	}

	return filterOptionalConstraints(
		[]gardencorev1beta1.Condition{hibernationPossibleConstraint, maintenancePreconditionsSatisfiedConstraint},
		[]gardencorev1beta1.Condition{caCertificateValiditiesAcceptableConstraint},
	)
}

var (
	notResourceManager   = utils.MustNewRequirement(v1beta1constants.LabelApp, selection.NotIn, resourcemanager.LabelValue)
	notManagedByGardener = utils.MustNewRequirement(resourcesv1alpha1.ManagedBy, selection.NotIn, "gardener")
	labelSelector        = client.MatchingLabelsSelector{Selector: labels.NewSelector().Add(notResourceManager).Add(notManagedByGardener)}
)

func getValidatingWebhookConfigurations(ctx context.Context, client client.Client) ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	validatingWebhookConfigs := &admissionregistrationv1.ValidatingWebhookConfigurationList{}
	if err := client.List(ctx, validatingWebhookConfigs, labelSelector); err != nil {
		return nil, err
	}
	return validatingWebhookConfigs.Items, nil
}

func getMutatingWebhookConfigurations(ctx context.Context, c client.Client) ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	mutatingWebhookConfigs := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := c.List(ctx, mutatingWebhookConfigs, labelSelector); err != nil {
		return nil, err
	}
	return mutatingWebhookConfigs.Items, nil
}

// CheckIfCACertificateValiditiesAcceptable checks whether there are CA certificates which are expiring in less than a
// year.
func (c *Constraint) CheckIfCACertificateValiditiesAcceptable(ctx context.Context) (gardencorev1beta1.ConditionStatus, string, string, []gardencorev1beta1.ErrorCode, error) {
	// CA certificates are valid for 10y, so let's only consider those certificates problematic which are valid for less
	// than 1y.
	const minimumValidity = 24 * time.Hour * 365

	secretList := &corev1.SecretList{}
	if err := c.seedClient.List(ctx, secretList, client.InNamespace(c.shoot.SeedNamespace), client.MatchingLabels{
		secretsmanager.LabelKeyManagedBy:       secretsmanager.LabelValueSecretsManager,
		secretsmanager.LabelKeyManagerIdentity: v1beta1constants.SecretManagerIdentityGardenlet,
		secretsmanager.LabelKeyPersist:         secretsmanager.LabelValueTrue,
	}); err != nil {
		return "", "", "", nil, fmt.Errorf("could not list secrets in shoot namespace in seed to check for expiring CA certificates: %w", err)
	}

	expiringCACertificates := make(map[string]time.Time, len(secretList.Items))
	for _, secret := range secretList.Items {
		if secret.Data[secretutils.DataKeyCertificateCA] == nil || secret.Data[secretutils.DataKeyPrivateKeyCA] == nil {
			continue
		}

		validUntilUnix, err := strconv.ParseInt(secret.Labels[secretsmanager.LabelKeyValidUntilTime], 10, 64)
		if err != nil {
			return "", "", "", nil, fmt.Errorf("could not parse %s label from secret %q: %w", secretsmanager.LabelKeyValidUntilTime, secret.Name, err)
		}
		validUntil := time.Unix(validUntilUnix, 0).UTC()

		if validUntil.Sub(c.clock.Now().UTC()) < minimumValidity {
			expiringCACertificates[secret.Labels[secretsmanager.LabelKeyName]] = validUntil
		}
	}

	if len(expiringCACertificates) > 0 {
		var msgs []string
		for name, validUntil := range expiringCACertificates {
			msgs = append(msgs, fmt.Sprintf("%q (expiring at %s)", name, validUntil))
		}

		return gardencorev1beta1.ConditionFalse,
			"ExpiringCACertificates",
			fmt.Sprintf("Some CA certificates are expiring in less than %s, you should rotate them: %s", minimumValidity, strings.Join(msgs, ", ")),
			nil,
			nil
	}

	return gardencorev1beta1.ConditionTrue,
		"NoExpiringCACertificates",
		fmt.Sprintf("All CA certificates are still valid for at least %s.", minimumValidity),
		nil,
		nil
}

// CheckForProblematicWebhooks checks the Shoot for problematic webhooks which could prevent shoot worker nodes from
// joining the cluster.
func (c *Constraint) CheckForProblematicWebhooks(ctx context.Context) (gardencorev1beta1.ConditionStatus, string, string, []gardencorev1beta1.ErrorCode, error) {
	validatingWebhookConfigs, err := getValidatingWebhookConfigurations(ctx, c.shootClient)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("could not get ValidatingWebhookConfigurations of Shoot cluster to check for problematic webhooks: %w", err)
	}

	for _, webhookConfig := range validatingWebhookConfigs {
		for _, w := range webhookConfig.Webhooks {
			if IsProblematicWebhook(w.FailurePolicy, w.ObjectSelector, w.NamespaceSelector, w.Rules, w.TimeoutSeconds) {
				msg := buildProblematicWebhookMessage("ValidatingWebhookConfiguration", webhookConfig.Name, w.Name, w.FailurePolicy, w.TimeoutSeconds)
				return gardencorev1beta1.ConditionFalse,
					"ProblematicWebhooks",
					msg,
					[]gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorProblematicWebhook},
					nil
			}
		}

		if wasRemediatedByGardener(webhookConfig.Annotations) {
			return gardencorev1beta1.ConditionFalse,
				"RemediatedWebhooks",
				fmt.Sprintf("ValidatingWebhookConfiguration %q is problematic and was remediated by Gardener (please check its annotations for details).", webhookConfig.Name),
				[]gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorProblematicWebhook},
				nil
		}
	}

	mutatingWebhookConfigs, err := getMutatingWebhookConfigurations(ctx, c.shootClient)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("could not get MutatingWebhookConfigurations of Shoot cluster to check for problematic webhooks: %w", err)
	}

	for _, webhookConfig := range mutatingWebhookConfigs {
		for _, w := range webhookConfig.Webhooks {
			if IsProblematicWebhook(w.FailurePolicy, w.ObjectSelector, w.NamespaceSelector, w.Rules, w.TimeoutSeconds) {
				msg := buildProblematicWebhookMessage("MutatingWebhookConfiguration", webhookConfig.Name, w.Name, w.FailurePolicy, w.TimeoutSeconds)
				return gardencorev1beta1.ConditionFalse,
					"ProblematicWebhooks",
					msg,
					[]gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorProblematicWebhook},
					nil
			}
		}

		if wasRemediatedByGardener(webhookConfig.Annotations) {
			return gardencorev1beta1.ConditionFalse,
				"RemediatedWebhooks",
				fmt.Sprintf("MutatingWebhookConfiguration %q is problematic and was remediated by Gardener (please check its annotations for details).", webhookConfig.Name),
				[]gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorProblematicWebhook},
				nil
		}
	}

	return gardencorev1beta1.ConditionTrue,
		"NoProblematicWebhooks",
		"All webhooks are properly configured.",
		nil,
		nil
}

func buildProblematicWebhookMessage(
	kind string,
	configName string,
	webhookName string,
	failurePolicy *admissionregistrationv1.FailurePolicyType,
	timeoutSeconds *int32,
) string {
	failurePolicyString := "nil"
	if failurePolicy != nil {
		failurePolicyString = string(*failurePolicy)
	}
	timeoutString := "and unset timeoutSeconds"
	if timeoutSeconds != nil {
		timeoutString = fmt.Sprintf("and %ds timeout", *timeoutSeconds)
	}
	return fmt.Sprintf("%s %q is problematic: webhook %q with failurePolicy %q %s might prevent worker nodes from properly joining the shoot cluster",
		kind, configName, webhookName, failurePolicyString, timeoutString)
}

// IsProblematicWebhook checks if a single webhook of the Shoot Cluster is problematic. Problematic webhooks are
// webhooks with rules for CREATE/UPDATE/* pods or nodes and failurePolicy=Fail/nil. If the Shoot contains such a
// webhook, we can never wake up this shoot cluster again as new nodes cannot get created/ready, or our system
// component pods cannot get created/ready (because the webhook's backing pod is not yet running).
func IsProblematicWebhook(
	failurePolicy *admissionregistrationv1.FailurePolicyType,
	objSelector *metav1.LabelSelector,
	nsSelector *metav1.LabelSelector,
	rules []admissionregistrationv1.RuleWithOperations,
	timeoutSeconds *int32,
) bool {
	if failurePolicy != nil && *failurePolicy != admissionregistrationv1.Fail {
		// in admissionregistration.k8s.io/v1 FailurePolicy is defaulted to `Fail`
		// see https://github.com/kubernetes/api/blob/release-1.16/admissionregistration/v1/types.go#L195
		// and https://github.com/kubernetes/api/blob/release-1.16/admissionregistration/v1/types.go#L324
		// therefore, webhook with FailurePolicy==nil is also considered problematic
		if timeoutSeconds != nil && *timeoutSeconds <= WebhookMaximumTimeoutSecondsNotProblematic {
			// most control-plane API calls are made with a client-side timeout of 30s, so if a webhook has
			// timeoutSeconds==30 the overall request might still fail although failurePolicy==Ignore, as there
			// is overhead in communication with the API server and possible other webhooks.
			// in admissionregistration/v1 timeoutSeconds is defaulted to 10 while in v1beta1 it's defaulted to 30.
			// be restrictive here and mark all webhooks without a timeout set or timeouts > 15s as problematic to
			// avoid ops effort. It's clearly documented that users should specify low timeouts, see
			// https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#timeouts
			return false
		}
	}

	for _, rule := range rules {
		for _, matcher := range matchers.WebhookConstraintMatchers {
			if matcher.Match(rule, objSelector, nsSelector) {
				return true
			}
		}
	}

	return false
}

func wasRemediatedByGardener(annotations map[string]string) bool {
	return annotations[v1beta1constants.GardenerWarning] != ""
}

func filterOptionalConstraints(required, optional []gardencorev1beta1.Condition) []gardencorev1beta1.Condition {
	var out []gardencorev1beta1.Condition
	out = append(out, required...)

	for _, constraint := range optional {
		if constraint.Status != gardencorev1beta1.ConditionTrue {
			out = append(out, constraint)
		}
	}

	return out
}
