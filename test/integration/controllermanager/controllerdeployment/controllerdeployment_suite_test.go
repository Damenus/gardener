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

package controllerdeployment_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
	controllerdeploymentcontroller "github.com/gardener/gardener/pkg/controllermanager/controller/controllerdeployment"
	gardenerenvtest "github.com/gardener/gardener/pkg/envtest"
	"github.com/gardener/gardener/pkg/logger"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestControllerDeploymentController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ControllerDeployment Controller Integration Test Suite")
}

const (
	// FinalizerName is the finalizer used by this controller.
	FinalizerName = "core.gardener.cloud/controllerdeployment"
	// testID is used for generating test namespace names and other IDs
	testID = "controllerdeployment-controller-test"
)

var (
	ctx = context.Background()
	log logr.Logger

	restConfig *rest.Config
	testEnv    *gardenerenvtest.GardenerTestEnvironment
	testClient client.Client
	mgrClient  client.Reader

	testNamespace *corev1.Namespace
	testRunID     string
)

var _ = BeforeSuite(func() {
	logf.SetLogger(logger.MustNewZapLogger(logger.DebugLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))
	log = logf.Log.WithName(testID)

	By("starting test environment")
	testEnv = &gardenerenvtest.GardenerTestEnvironment{
		GardenerAPIServer: &gardenerenvtest.GardenerAPIServer{},
	}

	var err error
	restConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	DeferCleanup(func() {
		By("stopping test environment")
		Expect(testEnv.Stop()).To(Succeed())
	})

	By("creating test client")
	testClient, err = client.New(restConfig, client.Options{Scheme: kubernetes.GardenScheme})
	Expect(err).NotTo(HaveOccurred())

	By("creating test namespace")
	testNamespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			// create dedicated namespace for each test run, so that we can run multiple tests concurrently for stress tests
			GenerateName: testID + "-",
		},
	}
	Expect(testClient.Create(ctx, testNamespace)).To(Succeed())
	log.Info("Created Namespace for test", "namespaceName", testNamespace.Name)
	testRunID = testNamespace.Name

	DeferCleanup(func() {
		By("deleting test namespace")
		Expect(testClient.Delete(ctx, testNamespace)).To(Or(Succeed(), BeNotFoundError()))
	})

	By("setting up manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Scheme:             kubernetes.GardenScheme,
		MetricsBindAddress: "0",
		Namespace:          testNamespace.Name,
		NewCache: cache.BuilderWithOptions(cache.Options{
			DefaultSelector: cache.ObjectSelector{
				Label: labels.SelectorFromSet(labels.Set{testID: testRunID}),
			},
		}),
	})
	mgrClient = mgr.GetClient()
	Expect(err).NotTo(HaveOccurred())

	By("registering controller")
	Expect((&controllerdeploymentcontroller.Reconciler{
		Config: config.ControllerDeploymentControllerConfiguration{
			ConcurrentSyncs: pointer.Int(5),
		},
		// limit exponential backoff in tests
		RateLimiter: workqueue.NewWithMaxWaitRateLimiter(workqueue.DefaultControllerRateLimiter(), 100*time.Millisecond),
	}).AddToManager(mgr)).To(Succeed())

	By("starting manager")
	mgrContext, mgrCancel := context.WithCancel(ctx)

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrContext)).To(Succeed())
	}()

	DeferCleanup(func() {
		By("stopping manager")
		mgrCancel()
	})
})
