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

package care_test

import (
	"context"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/features"
	gardenletfeatures "github.com/gardener/gardener/pkg/gardenlet/features"
	"github.com/gardener/gardener/pkg/operation/botanist/component/clusterautoscaler"
	"github.com/gardener/gardener/pkg/operation/botanist/component/clusteridentity"
	"github.com/gardener/gardener/pkg/operation/botanist/component/dependencywatchdog"
	"github.com/gardener/gardener/pkg/operation/botanist/component/etcd"
	"github.com/gardener/gardener/pkg/operation/botanist/component/hvpa"
	"github.com/gardener/gardener/pkg/operation/botanist/component/istio"
	"github.com/gardener/gardener/pkg/operation/botanist/component/kubestatemetrics"
	"github.com/gardener/gardener/pkg/operation/botanist/component/networkpolicies"
	"github.com/gardener/gardener/pkg/operation/botanist/component/nginxingress"
	"github.com/gardener/gardener/pkg/operation/botanist/component/seedadmissioncontroller"
	"github.com/gardener/gardener/pkg/operation/botanist/component/seedsystem"
	"github.com/gardener/gardener/pkg/operation/botanist/component/vpa"
	"github.com/gardener/gardener/pkg/operation/care"
	"github.com/gardener/gardener/pkg/utils/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	requiredManagedResources = []string{
		etcd.Druid,
		seedadmissioncontroller.Name,
		networkpolicies.ManagedResourceControlName,
		clusteridentity.ManagedResourceControlName,
		clusterautoscaler.ManagedResourceControlName,
		kubestatemetrics.ManagedResourceName,
		seedsystem.ManagedResourceName,
		vpa.ManagedResourceControlName,
		istio.ManagedResourceControlName,
	}

	optionalManagedResources = []string{
		dependencywatchdog.ManagedResourceDependencyWatchdogEndpoint,
		dependencywatchdog.ManagedResourceDependencyWatchdogProbe,
		nginxingress.ManagedResourceName,
		hvpa.ManagedResourceName,
	}
)

var _ = Describe("Seed health", func() {
	var (
		ctx context.Context
		c   client.Client

		seed *gardencorev1beta1.Seed

		seedSystemComponentsHealthyCondition gardencorev1beta1.Condition
	)

	BeforeEach(func() {
		defer test.WithFeatureGate(gardenletfeatures.FeatureGate, features.HVPA, true)()

		ctx = context.TODO()
		c = fakeclient.NewClientBuilder().WithScheme(kubernetes.SeedScheme).Build()

		seed = &gardencorev1beta1.Seed{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
			Spec: gardencorev1beta1.SeedSpec{
				Ingress: &gardencorev1beta1.Ingress{
					Controller: gardencorev1beta1.IngressController{
						Kind: "nginx",
					},
				},
				Settings: &gardencorev1beta1.SeedSettings{
					ShootDNS: &gardencorev1beta1.SeedSettingShootDNS{
						Enabled: true,
					},
					DependencyWatchdog: &gardencorev1beta1.SeedSettingDependencyWatchdog{
						Endpoint: &gardencorev1beta1.SeedSettingDependencyWatchdogEndpoint{
							Enabled: true,
						},
						Probe: &gardencorev1beta1.SeedSettingDependencyWatchdogProbe{
							Enabled: true,
						},
					},
				},
			},
		}

		seedSystemComponentsHealthyCondition = gardencorev1beta1.Condition{
			Type: gardencorev1beta1.SeedSystemComponentsHealthy,
		}
	})

	Describe("#CheckSeed", func() {
		Context("When all managed resources are deployed successfully", func() {
			JustBeforeEach(func() {
				for _, name := range append(requiredManagedResources, optionalManagedResources...) {
					Expect(c.Create(ctx, healthyManagedResource(name))).To(Succeed())
				}
			})

			It("should set SeedSystemComponentsHealthy condition to true", func() {
				healthCheck := care.NewHealthForSeed(seed, c)
				updatedConditions := healthCheck.CheckSeed(ctx, []gardencorev1beta1.Condition{seedSystemComponentsHealthyCondition}, nil)
				Expect(len(updatedConditions)).ToNot(BeZero())
				Expect(updatedConditions[0]).To(beConditionWithStatusReasonAndMessage(gardencorev1beta1.ConditionTrue, "SystemComponentsRunning", "All system components are healthy."))
			})
		})

		Context("When optional managed resources are turned off, and required resources are deployed successfully", func() {
			JustBeforeEach(func() {
				defer test.WithFeatureGate(gardenletfeatures.FeatureGate, features.HVPA, false)()
				seed.Spec.Ingress.Controller.Kind = "foo"
				seed.Spec.Settings.DependencyWatchdog.Endpoint.Enabled = false
				seed.Spec.Settings.DependencyWatchdog.Probe.Enabled = false

				for _, name := range requiredManagedResources {
					Expect(c.Create(ctx, healthyManagedResource(name))).To(Succeed())
				}
			})

			It("should set SeedSystemComponentsHealthy condition to true", func() {
				healthCheck := care.NewHealthForSeed(seed, c)
				updatedConditions := healthCheck.CheckSeed(ctx, []gardencorev1beta1.Condition{seedSystemComponentsHealthyCondition}, nil)
				Expect(len(updatedConditions)).ToNot(BeZero())
				Expect(updatedConditions[0]).To(beConditionWithStatusReasonAndMessage(gardencorev1beta1.ConditionTrue, "SystemComponentsRunning", "All system components are healthy."))
			})
		})

		Context("When there are issues with seed managed resources", func() {
			var (
				now time.Time

				tests = func(reason, message string) {
					It("should set SeedSystemComponentsHealthy condition to False if there is no Progressing threshold duration mapping", func() {
						healthCheck := care.NewHealthForSeed(seed, c)
						updatedConditions := healthCheck.CheckSeed(ctx, []gardencorev1beta1.Condition{seedSystemComponentsHealthyCondition}, nil)

						Expect(len(updatedConditions)).ToNot(BeZero())
						Expect(updatedConditions[0]).To(beConditionWithStatusReasonAndMessage(gardencorev1beta1.ConditionFalse, reason, message))
					})

					It("should set SeedSystemComponentsHealthy condition to Progressing if time is within threshold duration and condition is currently False", func() {
						defer test.WithVars(
							&care.Now, func() time.Time { return now.Add(30 * time.Second) },
						)()
						seedSystemComponentsHealthyCondition.Status = gardencorev1beta1.ConditionFalse

						healthCheck := care.NewHealthForSeed(seed, c)
						updatedConditions := healthCheck.CheckSeed(
							ctx,
							[]gardencorev1beta1.Condition{seedSystemComponentsHealthyCondition},
							map[gardencorev1beta1.ConditionType]time.Duration{gardencorev1beta1.SeedSystemComponentsHealthy: time.Minute},
						)

						Expect(len(updatedConditions)).ToNot(BeZero())
						Expect(updatedConditions[0]).To(beConditionWithStatusReasonAndMessage(gardencorev1beta1.ConditionProgressing, reason, message))
					})

					It("should set SeedSystemComponentsHealthy condition to Progressing if time is within threshold duration and condition is currently True", func() {
						defer test.WithVars(
							&care.Now, func() time.Time { return now.Add(30 * time.Second) },
						)()
						seedSystemComponentsHealthyCondition.Status = gardencorev1beta1.ConditionTrue

						healthCheck := care.NewHealthForSeed(seed, c)
						updatedConditions := healthCheck.CheckSeed(
							ctx,
							[]gardencorev1beta1.Condition{seedSystemComponentsHealthyCondition},
							map[gardencorev1beta1.ConditionType]time.Duration{gardencorev1beta1.SeedSystemComponentsHealthy: time.Minute},
						)

						Expect(len(updatedConditions)).ToNot(BeZero())
						Expect(updatedConditions[0]).To(beConditionWithStatusReasonAndMessage(gardencorev1beta1.ConditionProgressing, reason, message))
					})

					It("should set SeedSystemComponentsHealthy condition to false if Progressing threshold duration has expired", func() {
						defer test.WithVars(
							&care.Now, func() time.Time { return now.Add(90 * time.Second) },
						)()

						seedSystemComponentsHealthyCondition.Status = gardencorev1beta1.ConditionProgressing

						healthCheck := care.NewHealthForSeed(seed, c)
						updatedConditions := healthCheck.CheckSeed(
							ctx,
							[]gardencorev1beta1.Condition{seedSystemComponentsHealthyCondition},
							map[gardencorev1beta1.ConditionType]time.Duration{gardencorev1beta1.SeedSystemComponentsHealthy: time.Minute},
						)

						Expect(len(updatedConditions)).ToNot(BeZero())
						Expect(updatedConditions[0]).To(beConditionWithStatusReasonAndMessage(gardencorev1beta1.ConditionFalse, reason, message))
					})
				}
			)

			Context("When optional managed resources are enabled in seed settings but not deployed", func() {
				JustBeforeEach(func() {
					for _, name := range requiredManagedResources {
						Expect(c.Create(ctx, healthyManagedResource(name))).To(Succeed())
					}
				})

				tests("ResourceNotFound", "not found")
			})

			Context("When required managed resources are not deployed", func() {
				JustBeforeEach(func() {
					for _, name := range optionalManagedResources {
						Expect(c.Create(ctx, healthyManagedResource(name))).To(Succeed())
					}
				})

				tests("ResourceNotFound", "not found")
			})

			Context("When all managed resources are deployed, but not healthy", func() {
				JustBeforeEach(func() {
					for _, name := range append(requiredManagedResources, optionalManagedResources...) {
						Expect(c.Create(ctx, notHealthyManagedResource(name))).To(Succeed())
					}
				})

				tests("NotHealthy", "Resources are not healthy")
			})

			Context("When all managed resources are deployed but their resources are not applied", func() {
				JustBeforeEach(func() {
					for _, name := range append(requiredManagedResources, optionalManagedResources...) {
						Expect(c.Create(ctx, notAppliedManagedResource(name))).To(Succeed())
					}
				})

				tests("NotApplied", "Resources are not applied")
			})

			Context("When all managed resources are deployed but their resources are still progressing", func() {
				JustBeforeEach(func() {
					for _, name := range append(requiredManagedResources, optionalManagedResources...) {
						Expect(c.Create(ctx, progressingManagedResource(name))).To(Succeed())
					}
				})

				tests("ResourcesProgressing", "Resources are progressing")
			})

			Context("When all managed resources are deployed but not all required conditions are present", func() {
				JustBeforeEach(func() {
					for _, name := range append(requiredManagedResources, optionalManagedResources...) {
						Expect(c.Create(ctx, managedResource(name, []gardencorev1beta1.Condition{{
							Type:   resourcesv1alpha1.ResourcesApplied,
							Status: gardencorev1beta1.ConditionTrue}},
						))).To(Succeed())
					}
				})

				tests("MissingManagedResourceCondition", "is missing the following condition(s)")
			})
		})
	})
})

func beConditionWithStatusReasonAndMessage(status gardencorev1beta1.ConditionStatus, reason, message string) types.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(status),
		"Reason":  Equal(reason),
		"Message": ContainSubstring(message),
	})
}

func healthyManagedResource(name string) *resourcesv1alpha1.ManagedResource {
	return managedResource(
		name,
		[]gardencorev1beta1.Condition{
			{
				Type:   resourcesv1alpha1.ResourcesApplied,
				Status: gardencorev1beta1.ConditionTrue,
			},
			{
				Type:   resourcesv1alpha1.ResourcesHealthy,
				Status: gardencorev1beta1.ConditionTrue,
			},
			{
				Type:   resourcesv1alpha1.ResourcesProgressing,
				Status: gardencorev1beta1.ConditionFalse,
			},
		})
}

func notHealthyManagedResource(name string) *resourcesv1alpha1.ManagedResource {
	return managedResource(
		name,
		[]gardencorev1beta1.Condition{
			{
				Type:   resourcesv1alpha1.ResourcesApplied,
				Status: gardencorev1beta1.ConditionTrue,
			},
			{
				Type:    resourcesv1alpha1.ResourcesHealthy,
				Reason:  "NotHealthy",
				Message: "Resources are not healthy",
				Status:  gardencorev1beta1.ConditionFalse,
			},
			{
				Type:   resourcesv1alpha1.ResourcesProgressing,
				Status: gardencorev1beta1.ConditionFalse,
			},
		})
}

func notAppliedManagedResource(name string) *resourcesv1alpha1.ManagedResource {
	return managedResource(
		name,
		[]gardencorev1beta1.Condition{
			{
				Type:    resourcesv1alpha1.ResourcesApplied,
				Reason:  "NotApplied",
				Message: "Resources are not applied",
				Status:  gardencorev1beta1.ConditionFalse,
			},
			{
				Type:   resourcesv1alpha1.ResourcesHealthy,
				Status: gardencorev1beta1.ConditionTrue,
			},
			{
				Type:   resourcesv1alpha1.ResourcesProgressing,
				Status: gardencorev1beta1.ConditionFalse,
			},
		})
}

func progressingManagedResource(name string) *resourcesv1alpha1.ManagedResource {
	return managedResource(
		name,
		[]gardencorev1beta1.Condition{
			{
				Type:   resourcesv1alpha1.ResourcesApplied,
				Status: gardencorev1beta1.ConditionTrue,
			},
			{
				Type:   resourcesv1alpha1.ResourcesHealthy,
				Status: gardencorev1beta1.ConditionTrue,
			},
			{
				Type:    resourcesv1alpha1.ResourcesProgressing,
				Reason:  "ResourcesProgressing",
				Message: "Resources are progressing",
				Status:  gardencorev1beta1.ConditionTrue,
			},
		})
}

func managedResource(name string, conditions []gardencorev1beta1.Condition) *resourcesv1alpha1.ManagedResource {
	namespace := v1beta1constants.GardenNamespace
	if name == istio.ManagedResourceControlName {
		namespace = v1beta1constants.IstioSystemNamespace
	}

	return &resourcesv1alpha1.ManagedResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: resourcesv1alpha1.ManagedResourceStatus{
			Conditions: conditions,
		},
	}
}
