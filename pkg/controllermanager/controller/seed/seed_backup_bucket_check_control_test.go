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

package seed_test

import (
	"context"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
	. "github.com/gardener/gardener/pkg/controllermanager/controller/seed"
	backupbucketstrategy "github.com/gardener/gardener/pkg/registry/core/backupbucket"
	"github.com/gardener/gardener/pkg/utils/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclock "k8s.io/utils/clock/testing"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("BackupBucketsCheckReconciler", func() {
	const syncPeriod = 1 * time.Second

	var (
		ctx context.Context
		c   client.Client

		conf      config.SeedBackupBucketsCheckControllerConfiguration
		fakeClock *testclock.FakeClock

		expectedRequeueAfter time.Duration
	)

	Describe("#Reconcile", func() {
		var (
			seed          *gardencorev1beta1.Seed
			backupBuckets []gardencorev1beta1.BackupBucket

			reconciler reconcile.Reconciler
			request    reconcile.Request

			matchExpectedCondition types.GomegaMatcher
		)

		BeforeEach(func() {
			seed = &gardencorev1beta1.Seed{
				ObjectMeta: metav1.ObjectMeta{
					Name: "seed",
				},
			}

			request = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(seed)}

			fakeClock = testclock.NewFakeClock(time.Now().Round(time.Second))

			c = test.NewClientWithFieldSelectorSupport(
				fakeclient.NewClientBuilder().WithScheme(kubernetes.GardenScheme).WithObjects(seed).Build(),
				backupbucketstrategy.ToSelectableFields,
			)

			conf = config.SeedBackupBucketsCheckControllerConfiguration{
				SyncPeriod: &metav1.Duration{Duration: syncPeriod},
			}

			expectedRequeueAfter = syncPeriod
		})

		JustBeforeEach(func() {
			reconciler = NewBackupBucketsCheckReconciler(c, conf, fakeClock)

			for _, backupBucket := range backupBuckets {
				Expect(c.Create(ctx, &backupBucket)).To(Succeed())
			}
		})

		AfterEach(func() {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(seed)})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{RequeueAfter: expectedRequeueAfter}))

			if err := c.Get(ctx, request.NamespacedName, seed); !apierrors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
				Expect(seed.Status.Conditions).To(ConsistOf(matchExpectedCondition))
			}
		})

		It("should do nothing if Seed is gone", func() {
			Expect(c.Delete(ctx, seed)).To(Succeed())
			expectedRequeueAfter = 0
		})

		Context("when Seed has healthy backup buckets", func() {
			BeforeEach(func() {
				backupBuckets = []gardencorev1beta1.BackupBucket{
					createBackupBucket("1", seed.Name, nil),
					createBackupBucket("2", "fooSeed", nil),
					createBackupBucket("3", "barSeed", nil),
					createBackupBucket("4", seed.Name, nil),
				}
			})

			It("should set condition to `True` when none was given", func() {
				matchExpectedCondition = MatchFields(IgnoreExtras, Fields{
					"Message": Equal("Backup Buckets are available."),
					"Reason":  Equal("BackupBucketsAvailable"),
					"Status":  Equal(gardencorev1beta1.ConditionTrue),
					"Type":    Equal(gardencorev1beta1.SeedBackupBucketsReady),
				})
			})

			It("should set condition to `True` when it was false", func() {
				seed.Status.Conditions = []gardencorev1beta1.Condition{
					{
						Message: "foo",
						Reason:  "bar",
						Status:  gardencorev1beta1.ConditionFalse,
						Type:    gardencorev1beta1.SeedBackupBucketsReady,
					},
				}
				Expect(c.Update(ctx, seed)).To(Succeed())

				matchExpectedCondition = MatchFields(IgnoreExtras, Fields{
					"Message": Equal("Backup Buckets are available."),
					"Reason":  Equal("BackupBucketsAvailable"),
					"Status":  Equal(gardencorev1beta1.ConditionTrue),
					"Type":    Equal(gardencorev1beta1.SeedBackupBucketsReady),
				})
			})
		})

		Context("when there is a problem with the Seed's backup buckets", func() {
			var tests = func(expectedConditionStatus gardencorev1beta1.ConditionStatus, reason string, matchMessage types.GomegaMatcher) {
				It("should set correct condition status", func() {
					seed.Status.Conditions = []gardencorev1beta1.Condition{
						{
							Message: "Backup Buckets are available.",
							Reason:  "BackupBucketsAvailable",
							Status:  gardencorev1beta1.ConditionTrue,
							Type:    gardencorev1beta1.SeedBackupBucketsReady,
						},
					}
					Expect(c.Update(ctx, seed)).To(Succeed())

					matchExpectedCondition = MatchFields(IgnoreExtras, Fields{
						"Message": matchMessage,
						"Reason":  Equal(reason),
						"Status":  Equal(expectedConditionStatus),
						"Type":    Equal(gardencorev1beta1.SeedBackupBucketsReady),
					})
				})

				Context("when condition threshold is set", func() {
					BeforeEach(func() {
						conf = config.SeedBackupBucketsCheckControllerConfiguration{
							SyncPeriod: &metav1.Duration{Duration: syncPeriod},
							ConditionThresholds: []config.ConditionThreshold{{
								Type:     string(gardencorev1beta1.SeedBackupBucketsReady),
								Duration: metav1.Duration{Duration: time.Minute},
							}},
						}
					})

					It("should set condition to `Progressing`", func() {
						seed.Status.Conditions = []gardencorev1beta1.Condition{
							{
								Message:            "Backup Buckets are available.",
								Reason:             "BackupBucketsAvailable",
								Status:             gardencorev1beta1.ConditionTrue,
								Type:               gardencorev1beta1.SeedBackupBucketsReady,
								LastTransitionTime: metav1.Time{Time: fakeClock.Now().Add(-30 * time.Second)},
								LastUpdateTime:     metav1.Time{Time: fakeClock.Now().Add(-30 * time.Second)},
							},
						}
						Expect(c.Update(ctx, seed)).To(Succeed())

						matchExpectedCondition = MatchFields(IgnoreExtras, Fields{
							"Message": matchMessage,
							"Reason":  Equal(reason),
							"Status":  Equal(gardencorev1beta1.ConditionProgressing),
							"Type":    Equal(gardencorev1beta1.SeedBackupBucketsReady),
						})
					})

					It("should set correct condition status when condition threshold expires", func() {
						seed.Status.Conditions = []gardencorev1beta1.Condition{
							{
								Message:            "foo",
								Reason:             "BackupBucketsError",
								Status:             gardencorev1beta1.ConditionProgressing,
								Type:               gardencorev1beta1.SeedBackupBucketsReady,
								LastTransitionTime: metav1.Time{Time: fakeClock.Now().Add(-2 * time.Minute)},
								LastUpdateTime:     metav1.Time{Time: fakeClock.Now().Add(-2 * time.Minute)},
							},
						}
						Expect(c.Update(ctx, seed)).To(Succeed())

						matchExpectedCondition = MatchFields(IgnoreExtras, Fields{
							"Message": matchMessage,
							"Reason":  Equal(reason),
							"Status":  Equal(expectedConditionStatus),
							"Type":    Equal(gardencorev1beta1.SeedBackupBucketsReady),
						})
					})
				})
			}

			Context("when Seed has unhealthy backup buckets", func() {
				BeforeEach(func() {
					backupBuckets = []gardencorev1beta1.BackupBucket{
						createBackupBucket("1", seed.Name, &gardencorev1beta1.LastError{Description: "foo error"}),
						createBackupBucket("2", "fooSeed", nil),
						createBackupBucket("3", seed.Name, &gardencorev1beta1.LastError{Description: "bar error"}),
						createBackupBucket("4", "barSeed", nil),
					}
				})

				tests(gardencorev1beta1.ConditionFalse, "BackupBucketsError", SatisfyAll(ContainSubstring("Name: 1, Error: foo error"), ContainSubstring("Name: 3, Error: bar error")))
			})

			Context("when a Seed's backup buckets are gone", func() {
				BeforeEach(func() {
					backupBuckets = []gardencorev1beta1.BackupBucket{
						createBackupBucket("1", "fooSeed", &gardencorev1beta1.LastError{Description: "foo error"}),
						createBackupBucket("2", "barSeed", nil),
					}
				})

				tests(gardencorev1beta1.ConditionUnknown, "BackupBucketsGone", Equal("Backup Buckets are gone."))
			})

			Context("when a Seed's unhealthy backup bucket switches", func() {
				BeforeEach(func() {
					backupBuckets = []gardencorev1beta1.BackupBucket{
						createBackupBucket("1", seed.Name, &gardencorev1beta1.LastError{Description: "foo error"}),
						createBackupBucket("2", seed.Name, nil),
					}
				})

				It("should set condition to `False` and remove successful bucket from message", func() {
					seed.Status.Conditions = []gardencorev1beta1.Condition{
						{
							Message: "The following BackupBuckets have issues:\n* Name: 2, Error: some error",
							Reason:  "BackupBucketsError",
							Status:  gardencorev1beta1.ConditionFalse,
							Type:    gardencorev1beta1.SeedBackupBucketsReady,
						},
					}
					Expect(c.Update(ctx, seed)).To(Succeed())

					matchExpectedCondition = MatchFields(IgnoreExtras, Fields{
						"Message": Equal("The following BackupBuckets have issues:\n* Name: 1, Error: foo error"),
						"Type":    Equal(gardencorev1beta1.SeedBackupBucketsReady),
						"Status":  Equal(gardencorev1beta1.ConditionFalse),
					})
				})
			})
		})
	})
})

func createBackupBucket(name, seedName string, lastErr *gardencorev1beta1.LastError) gardencorev1beta1.BackupBucket {
	return gardencorev1beta1.BackupBucket{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: gardencorev1beta1.BackupBucketSpec{
			SeedName: pointer.String(seedName),
		},
		Status: gardencorev1beta1.BackupBucketStatus{
			LastError: lastErr,
		},
	}
}
