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
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ControllerInstallationCare controller tests", func() {
	var controllerInstallation *gardencorev1beta1.ControllerInstallation

	BeforeEach(func() {
		By("Create ControllerInstallation")
		controllerInstallation = &gardencorev1beta1.ControllerInstallation{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "foo-",
				Labels:       map[string]string{testID: testRunID},
			},
			Spec: gardencorev1beta1.ControllerInstallationSpec{
				SeedRef: corev1.ObjectReference{
					Name: "foo-seed",
				},
				RegistrationRef: corev1.ObjectReference{
					Name: "foo-registration",
				},
				DeploymentRef: &corev1.ObjectReference{
					Name: "foo-deployment",
				},
			},
		}
		Expect(testClient.Create(ctx, controllerInstallation)).To(Succeed())
		log.Info("Created controllerinstallation for test", "controllerinstallation", client.ObjectKeyFromObject(controllerInstallation))

		DeferCleanup(func() {
			By("Delete ControllerInstallation")
			Expect(testClient.Delete(ctx, controllerInstallation)).To(Succeed())
		})
	})

	Context("when ManagedResources for the ControllerInstallation does not exist", func() {
		It("should set conditions to Unknown", func() {
			Eventually(func(g Gomega) {
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(controllerInstallation), controllerInstallation)).To(Succeed())
				g.Expect(controllerInstallation.Status.Conditions).To(ConsistOf(
					And(ofType(gardencorev1beta1.ControllerInstallationInstalled), withStatus(gardencorev1beta1.ConditionUnknown), withReason("SeedReadError"), withMessageSubstrings("Failed to get ManagedResource", "not found")),
					And(ofType(gardencorev1beta1.ControllerInstallationHealthy), withStatus(gardencorev1beta1.ConditionUnknown), withReason("SeedReadError"), withMessageSubstrings("Failed to get ManagedResource", "not found")),
					And(ofType(gardencorev1beta1.ControllerInstallationProgressing), withStatus(gardencorev1beta1.ConditionUnknown), withReason("SeedReadError"), withMessageSubstrings("Failed to get ManagedResource", "not found")),
				))
			}).Should(Succeed())
		})
	})

	Context("when ManagedResource for the ControllerInstallation exists", func() {
		var managedResource *resourcesv1alpha1.ManagedResource

		BeforeEach(func() {
			By("Create ManagedResource")
			managedResource = &resourcesv1alpha1.ManagedResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:       controllerInstallation.Name,
					Namespace:  gardenNamespace.Name,
					Generation: 1,
				},
				Spec: resourcesv1alpha1.ManagedResourceSpec{
					SecretRefs: []corev1.LocalObjectReference{{
						Name: "foo-secret",
					}},
				},
			}
			Expect(testClient.Create(ctx, managedResource)).To(Succeed())
			log.Info("Created managedresource for test", "managedresource", client.ObjectKeyFromObject(managedResource))

			DeferCleanup(func() {
				By("Delete ManagedResource")
				Expect(testClient.Delete(ctx, managedResource)).To(Succeed())
			})
		})

		Context("when generation of ManagedResource is outdated", func() {
			It("shout set Installed condition to False with generation outdated error", func() {
				Eventually(func(g Gomega) {
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(controllerInstallation), controllerInstallation)).To(Succeed())
					g.Expect(controllerInstallation.Status.Conditions).To(containCondition(ofType(gardencorev1beta1.ControllerInstallationInstalled), withStatus(gardencorev1beta1.ConditionFalse), withReason("InstallationPending"), withMessageSubstrings("observed generation of managed resource", "outdated (0/1)")))
				}).Should(Succeed())
			})
		})

		Context("when generation of ManagedResource is up to date", func() {
			BeforeEach(func() {
				managedResource.Status.ObservedGeneration = managedResource.Generation
				Expect(testClient.Status().Update(ctx, managedResource)).To(Succeed())
			})

			It("should set conditions to failed when ManagedResource conditions do not exist yet", func() {
				Eventually(func(g Gomega) {
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(controllerInstallation), controllerInstallation)).To(Succeed())
					g.Expect(controllerInstallation.Status.Conditions).To(ConsistOf(
						And(ofType(gardencorev1beta1.ControllerInstallationInstalled), withStatus(gardencorev1beta1.ConditionFalse), withReason("InstallationPending"), withMessageSubstrings("condition", "has not been reported")),
						And(ofType(gardencorev1beta1.ControllerInstallationHealthy), withStatus(gardencorev1beta1.ConditionFalse), withReason("ControllerNotHealthy"), withMessageSubstrings("condition", "has not been reported")),
						And(ofType(gardencorev1beta1.ControllerInstallationProgressing), withStatus(gardencorev1beta1.ConditionTrue), withReason("ControllerNotRolledOut"), withMessageSubstrings("condition", "has not been reported")),
					))
				}).Should(Succeed())
			})

			It("should set conditions to failed when conditions of ManagedResource are not successful yet", func() {
				managedResource.Status.Conditions = []gardencorev1beta1.Condition{
					{Type: resourcesv1alpha1.ResourcesApplied, Status: gardencorev1beta1.ConditionFalse, LastTransitionTime: metav1.Now(), LastUpdateTime: metav1.Now()},
					{Type: resourcesv1alpha1.ResourcesHealthy, Status: gardencorev1beta1.ConditionFalse, LastTransitionTime: metav1.Now(), LastUpdateTime: metav1.Now()},
					{Type: resourcesv1alpha1.ResourcesProgressing, Status: gardencorev1beta1.ConditionTrue, LastTransitionTime: metav1.Now(), LastUpdateTime: metav1.Now()},
				}
				Expect(testClient.Status().Update(ctx, managedResource)).To(Succeed())

				Eventually(func(g Gomega) {
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(controllerInstallation), controllerInstallation)).To(Succeed())
					g.Expect(controllerInstallation.Status.Conditions).To(ConsistOf(
						And(ofType(gardencorev1beta1.ControllerInstallationInstalled), withStatus(gardencorev1beta1.ConditionFalse), withReason("InstallationPending")),
						And(ofType(gardencorev1beta1.ControllerInstallationHealthy), withStatus(gardencorev1beta1.ConditionFalse), withReason("ControllerNotHealthy")),
						And(ofType(gardencorev1beta1.ControllerInstallationProgressing), withStatus(gardencorev1beta1.ConditionTrue), withReason("ControllerNotRolledOut")),
					))
				}).Should(Succeed())
			})

			It("should set conditions to successful when conditions of ManagedResource become successful", func() {
				managedResource.Status.Conditions = []gardencorev1beta1.Condition{
					{Type: resourcesv1alpha1.ResourcesApplied, Status: gardencorev1beta1.ConditionTrue, LastTransitionTime: metav1.Now(), LastUpdateTime: metav1.Now()},
					{Type: resourcesv1alpha1.ResourcesHealthy, Status: gardencorev1beta1.ConditionTrue, LastTransitionTime: metav1.Now(), LastUpdateTime: metav1.Now()},
					{Type: resourcesv1alpha1.ResourcesProgressing, Status: gardencorev1beta1.ConditionFalse, LastTransitionTime: metav1.Now(), LastUpdateTime: metav1.Now()},
				}
				Expect(testClient.Status().Update(ctx, managedResource)).To(Succeed())

				Eventually(func(g Gomega) {
					g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(controllerInstallation), controllerInstallation)).To(Succeed())
					g.Expect(controllerInstallation.Status.Conditions).To(ConsistOf(
						And(ofType(gardencorev1beta1.ControllerInstallationInstalled), withStatus(gardencorev1beta1.ConditionTrue), withReason("InstallationSuccessful")),
						And(ofType(gardencorev1beta1.ControllerInstallationHealthy), withStatus(gardencorev1beta1.ConditionTrue), withReason("ControllerHealthy")),
						And(ofType(gardencorev1beta1.ControllerInstallationProgressing), withStatus(gardencorev1beta1.ConditionFalse), withReason("ControllerRolledOut")),
					))
				}).Should(Succeed())
			})
		})
	})
})

func containCondition(matchers ...gomegatypes.GomegaMatcher) gomegatypes.GomegaMatcher {
	return ContainElement(And(matchers...))
}

func ofType(conditionType gardencorev1beta1.ConditionType) gomegatypes.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Type": Equal(conditionType),
	})
}

func withStatus(status gardencorev1beta1.ConditionStatus) gomegatypes.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Status": Equal(status),
	})
}

func withReason(reason string) gomegatypes.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Reason": Equal(reason),
	})
}

func withMessageSubstrings(messages ...string) gomegatypes.GomegaMatcher {
	var substringMatchers = make([]gomegatypes.GomegaMatcher, 0, len(messages))
	for _, message := range messages {
		substringMatchers = append(substringMatchers, ContainSubstring(message))
	}
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Message": SatisfyAll(substringMatchers...),
	})
}
