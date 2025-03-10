// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package project

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
	"github.com/gardener/gardener/pkg/controllerutils"
)

// ControllerName is the name of this controller.
const ControllerName = "project"

// Controller controls Projects.
type Controller struct {
	cache client.Reader
	log   logr.Logger

	clock clock.Clock

	projectReconciler         reconcile.Reconciler
	projectStaleReconciler    reconcile.Reconciler
	projectActivityReconciler reconcile.Reconciler
	hasSyncedFuncs            []cache.InformerSynced

	projectQueue         workqueue.RateLimitingInterface
	projectStaleQueue    workqueue.RateLimitingInterface
	projectActivityQueue workqueue.RateLimitingInterface

	workerCh               chan int
	numberOfRunningWorkers int
}

// NewProjectController takes a Kubernetes client for the Garden clusters <k8sGardenClient>, a struct
// holding information about the acting Gardener, a <projectInformer>, and a <recorder> for
// event recording. It creates a new Gardener controller.
func NewProjectController(
	ctx context.Context,
	log logr.Logger,
	mgr manager.Manager,
	config *config.ControllerManagerConfiguration,
) (
	*Controller,
	error,
) {
	log = log.WithName(ControllerName)

	gardenClient := mgr.GetClient()
	gardenCache := mgr.GetCache()

	projectInformer, err := gardenCache.GetInformer(ctx, &gardencorev1beta1.Project{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Project Informer: %w", err)
	}
	roleBindingInformer, err := gardenCache.GetInformer(ctx, &rbacv1.RoleBinding{})
	if err != nil {
		return nil, fmt.Errorf("failed to get RoleBinding Informer: %w", err)
	}
	shootInformer, err := gardenCache.GetInformer(ctx, &gardencorev1beta1.Shoot{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Shoot Informer: %w", err)
	}
	secretInformer, err := gardenCache.GetInformer(ctx, &corev1.Secret{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret Informer: %w", err)
	}
	backupEntryInformer, err := gardenCache.GetInformer(ctx, &gardencorev1beta1.BackupEntry{})
	if err != nil {
		return nil, fmt.Errorf("failed to get BackupEntry Informer: %w", err)
	}
	quotaInformer, err := gardenCache.GetInformer(ctx, &gardencorev1beta1.Quota{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Quota Informer: %w", err)
	}

	projectController := &Controller{
		cache:                     gardenCache,
		log:                       log,
		clock:                     &clock.RealClock{},
		projectReconciler:         NewProjectReconciler(config.Controllers.Project, gardenClient, mgr.GetEventRecorderFor(ControllerName+"-controller")),
		projectStaleReconciler:    NewProjectStaleReconciler(config.Controllers.Project, gardenClient),
		projectActivityReconciler: NewActivityReconciler(gardenClient, &clock.RealClock{}),
		projectQueue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Project"),
		projectStaleQueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Project Stale"),
		projectActivityQueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Project Activity"),
		workerCh:                  make(chan int),
	}

	projectInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    projectController.projectAdd,
		UpdateFunc: projectController.projectUpdate,
		DeleteFunc: projectController.projectDelete,
	})

	roleBindingInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) { projectController.roleBindingUpdate(ctx, oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { projectController.roleBindingDelete(ctx, obj) },
	})

	shootInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, false, true)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			projectController.projectActivityObjectUpdate(ctx, oldObj, newObj, false)
		},
		DeleteFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, false, false)
		},
	})

	secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, true, true)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			projectController.projectActivityObjectUpdate(ctx, oldObj, newObj, true)
		},
		DeleteFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, true, false)
		},
	})

	quotaInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, true, true)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			projectController.projectActivityObjectUpdate(ctx, oldObj, newObj, true)
		},
		DeleteFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, true, false)
		},
	})

	backupEntryInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, false, true)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			projectController.projectActivityObjectUpdate(ctx, oldObj, newObj, false)
		},
		DeleteFunc: func(obj interface{}) {
			projectController.projectActivityObjectAddDelete(ctx, obj, false, false)
		},
	})

	projectController.hasSyncedFuncs = append(projectController.hasSyncedFuncs,
		projectInformer.HasSynced,
		roleBindingInformer.HasSynced,
		shootInformer.HasSynced,
		secretInformer.HasSynced,
		backupEntryInformer.HasSynced,
		quotaInformer.HasSynced,
	)

	return projectController, nil
}

// Run runs the Controller until the given stop channel can be read from.
func (c *Controller) Run(ctx context.Context, workers int) {
	var waitGroup sync.WaitGroup

	if !cache.WaitForCacheSync(ctx.Done(), c.hasSyncedFuncs...) {
		c.log.Error(wait.ErrWaitTimeout, "Timed out waiting for caches to sync")
		return
	}

	// Count number of running workers.
	go func() {
		for res := range c.workerCh {
			c.numberOfRunningWorkers += res
		}
	}()

	c.log.Info("Project controller initialized")

	for i := 0; i < workers; i++ {
		controllerutils.CreateWorker(ctx, c.projectQueue, "Project", c.projectReconciler, &waitGroup, c.workerCh, controllerutils.WithLogger(c.log.WithName(projectReconcilerName)))
		controllerutils.CreateWorker(ctx, c.projectStaleQueue, "Project Stale", c.projectStaleReconciler, &waitGroup, c.workerCh, controllerutils.WithLogger(c.log.WithName(staleReconcilerName)))
		controllerutils.CreateWorker(ctx, c.projectActivityQueue, "Project Activity", c.projectActivityReconciler, &waitGroup, c.workerCh, controllerutils.WithLogger(c.log.WithName(projectActivityReconcilerName)))
	}

	// Shutdown handling
	<-ctx.Done()
	c.projectQueue.ShutDown()
	c.projectStaleQueue.ShutDown()
	c.projectActivityQueue.ShutDown()

	for {
		if c.projectQueue.Len() == 0 &&
			c.projectStaleQueue.Len() == 0 &&
			c.projectActivityQueue.Len() == 0 &&
			c.numberOfRunningWorkers == 0 {
			c.log.V(1).Info("No running Project worker and no items left in the queues. Terminating Project controller")
			break
		}
		c.log.V(1).Info(
			"Waiting for Project workers to finish",
			"numberOfRunningWorkers", c.numberOfRunningWorkers,
			"queueLength", c.projectQueue.Len()+c.projectStaleQueue.Len()+c.projectActivityQueue.Len(),
		)
		time.Sleep(5 * time.Second)
	}

	waitGroup.Wait()
}
