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

package backupbucketscheck_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

const (
	conditionThreshold = 1 * time.Second
	syncPeriod         = 1 * time.Millisecond
)

var _ = Describe("Seed BackupBucketsCheck controller tests", func() {
	var (
		seed *gardencorev1beta1.Seed
		bb1  *gardencorev1beta1.BackupBucket
		bb2  *gardencorev1beta1.BackupBucket
	)

	BeforeEach(func() {
		By("Create Seed")
		seed = &gardencorev1beta1.Seed{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: testID + "-",
				Labels:       map[string]string{testID: testRunID},
			},
			Spec: gardencorev1beta1.SeedSpec{
				Provider: gardencorev1beta1.SeedProvider{
					Region: "region",
					Type:   "providerType",
				},
				Settings: &gardencorev1beta1.SeedSettings{
					ShootDNS:   &gardencorev1beta1.SeedSettingShootDNS{Enabled: true},
					Scheduling: &gardencorev1beta1.SeedSettingScheduling{Visible: true},
				},
				Networks: gardencorev1beta1.SeedNetworks{
					Pods:     "10.0.0.0/16",
					Services: "10.1.0.0/16",
					Nodes:    pointer.String("10.2.0.0/16"),
					ShootDefaults: &gardencorev1beta1.ShootNetworks{
						Pods:     pointer.String("100.128.0.0/11"),
						Services: pointer.String("100.72.0.0/13"),
					},
				},
				DNS: gardencorev1beta1.SeedDNS{
					IngressDomain: pointer.String("someingress.example.com"),
				},
			},
		}
		Expect(testClient.Create(ctx, seed)).To(Succeed())
		log.Info("Created seed for test", "seed", client.ObjectKeyFromObject(seed))

		DeferCleanup(func() {
			By("Delete Seed")
			Expect(client.IgnoreNotFound(testClient.Delete(ctx, seed))).To(Succeed())
		})

		By("Create BackupBuckets")
		bb1 = &gardencorev1beta1.BackupBucket{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "foo-1-",
				Labels: map[string]string{
					"provider.extensions.gardener.cloud/providerType": "true",
					testID: testRunID,
				},
			},
			Spec: gardencorev1beta1.BackupBucketSpec{
				SeedName: &seed.Name,
				Provider: gardencorev1beta1.BackupBucketProvider{
					Type:   "providerType",
					Region: "region",
				},
				SecretRef: corev1.SecretReference{
					Name:      "secretName",
					Namespace: "garden",
				},
			},
		}

		bb2 = bb1.DeepCopy()
		bb2.SetGenerateName("foo-2-")

		for _, backupBucket := range []*gardencorev1beta1.BackupBucket{bb1, bb2} {
			Expect(testClient.Create(ctx, backupBucket)).To(Succeed())
			log.Info("Created backupbucket for test", "backupbucket", client.ObjectKeyFromObject(backupBucket))
		}

		DeferCleanup(func() {
			By("Delete BackupBuckets")
			for _, backupBucket := range []*gardencorev1beta1.BackupBucket{bb1, bb2} {
				Expect(client.IgnoreNotFound(testClient.Delete(ctx, backupBucket))).To(Succeed())
			}
		})

		By("waiting until BackupBucketsReady condition is set to True")
		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(seed), seed)).To(Succeed())
			g.Expect(seed.Status.Conditions).To(containCondition(ofType(gardencorev1beta1.SeedBackupBucketsReady), withStatus(gardencorev1beta1.ConditionTrue), withReason("BackupBucketsAvailable")))
		}).Should(Succeed())
	})

	var tests = func(expectedConditionStatus gardencorev1beta1.ConditionStatus, reason string) {
		It("should set BackupBucketsReady to Progressing and eventually to expected status when condition threshold expires", func() {
			Eventually(func(g Gomega) {
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(seed), seed)).To(Succeed())
				g.Expect(seed.Status.Conditions).To(containCondition(ofType(gardencorev1beta1.SeedBackupBucketsReady), withStatus(gardencorev1beta1.ConditionProgressing), withReason(reason)))
			}).Should(Succeed())

			fakeClock.Step(conditionThreshold + 1*time.Second)
			Eventually(func(g Gomega) {
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(seed), seed)).To(Succeed())
				g.Expect(seed.Status.Conditions).To(containCondition(ofType(gardencorev1beta1.SeedBackupBucketsReady), withStatus(expectedConditionStatus), withReason(reason)))
			}).Should(Succeed())
		})
	}

	Context("when one BackupBucket becomes erroneous", func() {
		BeforeEach(func() {
			bb1.Status.LastError = &gardencorev1beta1.LastError{
				Description: "foo",
			}
			Expect(testClient.Status().Update(ctx, bb1)).To(Succeed())
		})

		tests(gardencorev1beta1.ConditionFalse, "BackupBucketsError")
	})

	Context("when BackupBuckets for the Seed are gone", func() {
		BeforeEach(func() {
			for _, backupBucket := range []*gardencorev1beta1.BackupBucket{bb1, bb2} {
				Expect(client.IgnoreNotFound(testClient.Delete(ctx, backupBucket))).To(Succeed())
				Eventually(func() error {
					return testClient.Get(ctx, client.ObjectKeyFromObject(backupBucket), backupBucket)
				}).Should(BeNotFoundError())
			}
		})

		tests(gardencorev1beta1.ConditionUnknown, "BackupBucketsGone")
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
