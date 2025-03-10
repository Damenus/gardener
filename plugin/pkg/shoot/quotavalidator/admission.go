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

package quotavalidator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/core/helper"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	admissioninitializer "github.com/gardener/gardener/pkg/apiserver/admission/initializer"
	coreinformers "github.com/gardener/gardener/pkg/client/core/informers/internalversion"
	corelisters "github.com/gardener/gardener/pkg/client/core/listers/core/internalversion"
	utiltime "github.com/gardener/gardener/pkg/utils/time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/admission"
)

const (
	// PluginName is the name of this admission plugin.
	PluginName = "ShootQuotaValidator"
)

var (
	quotaMetricNames = [6]corev1.ResourceName{
		core.QuotaMetricCPU,
		core.QuotaMetricGPU,
		core.QuotaMetricMemory,
		core.QuotaMetricStorageStandard,
		core.QuotaMetricStoragePremium,
		core.QuotaMetricLoadbalancer}
)

// Register registers a plugin.
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return New(utiltime.DefaultOps())
	})
}

// QuotaValidator contains listers and and admission handler.
type QuotaValidator struct {
	*admission.Handler
	shootLister         corelisters.ShootLister
	cloudProfileLister  corelisters.CloudProfileLister
	secretBindingLister corelisters.SecretBindingLister
	quotaLister         corelisters.QuotaLister
	readyFunc           admission.ReadyFunc
	time                utiltime.Ops
}

var (
	_ = admissioninitializer.WantsInternalCoreInformerFactory(&QuotaValidator{})

	readyFuncs = []admission.ReadyFunc{}
)

// New creates a new QuotaValidator admission plugin.
func New(time utiltime.Ops) (*QuotaValidator, error) {
	return &QuotaValidator{
		Handler: admission.NewHandler(admission.Create, admission.Update),
		time:    time,
	}, nil
}

// AssignReadyFunc assigns the ready function to the admission handler.
func (q *QuotaValidator) AssignReadyFunc(f admission.ReadyFunc) {
	q.readyFunc = f
	q.SetReadyFunc(f)
}

// SetInternalCoreInformerFactory gets Lister from SharedInformerFactory.
func (q *QuotaValidator) SetInternalCoreInformerFactory(f coreinformers.SharedInformerFactory) {
	shootInformer := f.Core().InternalVersion().Shoots()
	q.shootLister = shootInformer.Lister()

	cloudProfileInformer := f.Core().InternalVersion().CloudProfiles()
	q.cloudProfileLister = cloudProfileInformer.Lister()

	secretBindingInformer := f.Core().InternalVersion().SecretBindings()
	q.secretBindingLister = secretBindingInformer.Lister()

	quotaInformer := f.Core().InternalVersion().Quotas()
	q.quotaLister = quotaInformer.Lister()

	readyFuncs = append(readyFuncs, shootInformer.Informer().HasSynced, cloudProfileInformer.Informer().HasSynced, secretBindingInformer.Informer().HasSynced, quotaInformer.Informer().HasSynced)
}

// ValidateInitialization checks whether the plugin was correctly initialized.
func (q *QuotaValidator) ValidateInitialization() error {
	if q.shootLister == nil {
		return errors.New("missing shoot lister")
	}
	if q.cloudProfileLister == nil {
		return errors.New("missing cloudProfile lister")
	}
	if q.secretBindingLister == nil {
		return errors.New("missing secretBinding lister")
	}
	if q.quotaLister == nil {
		return errors.New("missing quota lister")
	}
	return nil
}

var _ admission.ValidationInterface = &QuotaValidator{}

// Validate checks that the requested Shoot resources do not exceed the quota limits.
func (q *QuotaValidator) Validate(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	// Wait until the caches have been synced
	if q.readyFunc == nil {
		q.AssignReadyFunc(func() bool {
			for _, readyFunc := range readyFuncs {
				if !readyFunc() {
					return false
				}
			}
			return true
		})
	}
	if !q.WaitForReady() {
		return admission.NewForbidden(a, errors.New("not yet ready to handle request"))
	}

	// Ignore all kinds other than Shoot
	if a.GetKind().GroupKind() != core.Kind("Shoot") {
		return nil
	}
	if a.GetSubresource() != "" {
		return nil
	}

	shoot, ok := a.GetObject().(*core.Shoot)
	if !ok {
		return apierrors.NewBadRequest("could not convert resource into Shoot object")
	}

	// Pass if the shoot is intended to get deleted
	if shoot.DeletionTimestamp != nil {
		return nil
	}

	var (
		oldShoot         *core.Shoot
		maxShootLifetime *int32
		checkLifetime    = false
		checkQuota       = false
	)

	if a.GetOperation() == admission.Create {
		checkQuota = true
	}

	if a.GetOperation() == admission.Update {
		oldShoot, ok = a.GetOldObject().(*core.Shoot)
		if !ok {
			return apierrors.NewBadRequest("could not convert resource into Shoot object")
		}

		checkQuota = quotaVerificationNeeded(*shoot, *oldShoot)
		checkLifetime = lifetimeVerificationNeeded(*shoot, *oldShoot)
	}

	secretBinding, err := q.secretBindingLister.SecretBindings(shoot.Namespace).Get(shoot.Spec.SecretBindingName)
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	// Quotas are cumulative, means each quota must be not exceeded that the admission pass.
	for _, quotaRef := range secretBinding.Quotas {
		quota, err := q.quotaLister.Quotas(quotaRef.Namespace).Get(quotaRef.Name)
		if err != nil {
			return apierrors.NewInternalError(err)
		}

		// Get the max clusterLifeTime
		if checkLifetime && quota.Spec.ClusterLifetimeDays != nil {
			if maxShootLifetime == nil {
				maxShootLifetime = quota.Spec.ClusterLifetimeDays
			}
			if *maxShootLifetime > *quota.Spec.ClusterLifetimeDays {
				maxShootLifetime = quota.Spec.ClusterLifetimeDays
			}
		}

		if checkQuota {
			exceededMetrics, err := q.isQuotaExceeded(*shoot, *quota)
			if err != nil {
				return apierrors.NewInternalError(err)
			}
			if exceededMetrics != nil {
				message := ""
				for _, metric := range *exceededMetrics {
					message = message + metric.String() + " "
				}
				return admission.NewForbidden(a, fmt.Errorf("quota limits exceeded. Unable to allocate further %s", message))
			}
		}
	}

	// Admit Shoot lifetime changes
	if lifetime, exists := shoot.Annotations[v1beta1constants.ShootExpirationTimestamp]; checkLifetime && exists && maxShootLifetime != nil {
		var (
			plannedExpirationTime     time.Time
			maxPossibleExpirationTime time.Time
		)

		plannedExpirationTime, err = time.Parse(time.RFC3339, lifetime)
		if err != nil {
			return apierrors.NewInternalError(err)
		}

		maxPossibleExpirationTime = q.time.Now().Add(time.Duration(*maxShootLifetime*24) * time.Hour)
		if plannedExpirationTime.After(maxPossibleExpirationTime) {
			return admission.NewForbidden(a, fmt.Errorf("requested shoot expiration time is too long. Can only be extended by %d day(s)", *maxShootLifetime))
		}
	}

	return nil
}

func (q *QuotaValidator) isQuotaExceeded(shoot core.Shoot, quota core.Quota) (*[]corev1.ResourceName, error) {
	allocatedResources, err := q.determineAllocatedResources(quota, shoot)
	if err != nil {
		return nil, err
	}
	requiredResources, err := q.determineRequiredResources(allocatedResources, shoot)
	if err != nil {
		return nil, err
	}

	exceededMetrics := make([]corev1.ResourceName, 0)
	for _, metric := range quotaMetricNames {
		if _, ok := quota.Spec.Metrics[metric]; !ok {
			continue
		}
		if !hasSufficientQuota(quota.Spec.Metrics[metric], requiredResources[metric]) {
			exceededMetrics = append(exceededMetrics, metric)
		}
	}
	if len(exceededMetrics) != 0 {
		return &exceededMetrics, nil
	}
	return nil, nil
}

func (q *QuotaValidator) determineAllocatedResources(quota core.Quota, shoot core.Shoot) (corev1.ResourceList, error) {
	shoots, err := q.findShootsReferQuota(quota, shoot)
	if err != nil {
		return nil, err
	}

	// Collect the resources which are allocated according to the shoot specs
	allocatedResources := make(corev1.ResourceList)
	for _, s := range shoots {
		shootResources, err := q.getShootResources(s)
		if err != nil {
			return nil, err
		}
		for _, metric := range quotaMetricNames {
			allocatedResources[metric] = sumQuantity(allocatedResources[metric], shootResources[metric])
		}
	}

	// TODO: We have to determine and add the amount of storage, which is allocated by manually created persistent volumes
	// and the count of loadbalancer, which are created due to manually created services of type loadbalancer

	return allocatedResources, nil
}

func (q *QuotaValidator) findShootsReferQuota(quota core.Quota, shoot core.Shoot) ([]core.Shoot, error) {
	var (
		shootsReferQuota []core.Shoot
		secretBindings   []core.SecretBinding
	)

	scope, err := helper.QuotaScope(quota.Spec.Scope)
	if err != nil {
		return nil, err
	}

	namespace := corev1.NamespaceAll
	if scope == "project" {
		namespace = shoot.Namespace
	}
	allSecretBindings, err := q.secretBindingLister.SecretBindings(namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, binding := range allSecretBindings {
		for _, quotaRef := range binding.Quotas {
			if quota.Name == quotaRef.Name && quota.Namespace == quotaRef.Namespace {
				secretBindings = append(secretBindings, *binding)
			}
		}
	}

	for _, binding := range secretBindings {
		shoots, err := q.shootLister.Shoots(binding.Namespace).List(labels.Everything())
		if err != nil {
			return nil, err
		}
		for _, s := range shoots {
			if shoot.Namespace == s.Namespace && shoot.Name == s.Name {
				continue
			}
			if s.Spec.SecretBindingName == binding.Name {
				shootsReferQuota = append(shootsReferQuota, *s)
			}
		}
	}
	return shootsReferQuota, nil
}

func (q *QuotaValidator) determineRequiredResources(allocatedResources corev1.ResourceList, shoot core.Shoot) (corev1.ResourceList, error) {
	shootResources, err := q.getShootResources(shoot)
	if err != nil {
		return nil, err
	}

	requiredResources := make(corev1.ResourceList)
	for _, metric := range quotaMetricNames {
		requiredResources[metric] = sumQuantity(allocatedResources[metric], shootResources[metric])
	}
	return requiredResources, nil
}

func (q *QuotaValidator) getShootResources(shoot core.Shoot) (corev1.ResourceList, error) {
	cloudProfile, err := q.cloudProfileLister.Get(shoot.Spec.CloudProfileName)
	if err != nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("could not find referenced cloud profile: %+v", err.Error()))
	}

	var (
		countLB      int64 = 1
		resources          = make(corev1.ResourceList)
		workers            = getShootWorkerResources(&shoot, cloudProfile)
		machineTypes       = cloudProfile.Spec.MachineTypes
		volumeTypes        = cloudProfile.Spec.VolumeTypes
	)

	for _, worker := range workers {
		var (
			machineType *core.MachineType
			volumeType  *core.VolumeType
		)

		// Get the proper machineType
		for _, element := range machineTypes {
			if element.Name == worker.Machine.Type {
				machineType = &element
				break
			}
		}
		if machineType == nil {
			return nil, fmt.Errorf("machineType %s not found in CloudProfile %s", worker.Machine.Type, cloudProfile.Name)
		}

		if worker.Volume != nil {
			if machineType.Storage != nil {
				volumeType = &core.VolumeType{
					Class: machineType.Storage.Class,
				}
			} else {
				// Get the proper VolumeType
				for _, element := range volumeTypes {
					if worker.Volume.Type != nil && element.Name == *worker.Volume.Type {
						volumeType = &element
						break
					}
				}
			}
		}
		if volumeType == nil {
			return nil, fmt.Errorf("VolumeType %s not found in CloudProfile %s", worker.Machine.Type, cloudProfile.Name)
		}

		// For now we always use the max. amount of resources for quota calculation
		resources[core.QuotaMetricCPU] = sumQuantity(resources[core.QuotaMetricCPU], multiplyQuantity(machineType.CPU, worker.Maximum))
		resources[core.QuotaMetricGPU] = sumQuantity(resources[core.QuotaMetricGPU], multiplyQuantity(machineType.GPU, worker.Maximum))
		resources[core.QuotaMetricMemory] = sumQuantity(resources[core.QuotaMetricMemory], multiplyQuantity(machineType.Memory, worker.Maximum))

		size, _ := resource.ParseQuantity("0Gi")
		if worker.Volume != nil {
			size, err = resource.ParseQuantity(worker.Volume.VolumeSize)
			if err != nil {
				return nil, err
			}
		}

		switch volumeType.Class {
		case core.VolumeClassStandard:
			resources[core.QuotaMetricStorageStandard] = sumQuantity(resources[core.QuotaMetricStorageStandard], multiplyQuantity(size, worker.Maximum))
		case core.VolumeClassPremium:
			resources[core.QuotaMetricStoragePremium] = sumQuantity(resources[core.QuotaMetricStoragePremium], multiplyQuantity(size, worker.Maximum))
		default:
			return nil, fmt.Errorf("unknown volumeType class %s", volumeType.Class)
		}
	}

	if helper.NginxIngressEnabled(shoot.Spec.Addons) {
		countLB++
	}
	resources[core.QuotaMetricLoadbalancer] = *resource.NewQuantity(countLB, resource.DecimalSI)

	return resources, nil
}

func getShootWorkerResources(shoot *core.Shoot, cloudProfile *core.CloudProfile) []core.Worker {
	workers := make([]core.Worker, 0, len(shoot.Spec.Provider.Workers))

	for _, worker := range shoot.Spec.Provider.Workers {
		workerCopy := worker.DeepCopy()

		if worker.Volume == nil {
			for _, machineType := range cloudProfile.Spec.MachineTypes {
				if worker.Machine.Type == machineType.Name && machineType.Storage != nil && machineType.Storage.StorageSize != nil {
					workerCopy.Volume = &core.Volume{
						Type:       &machineType.Storage.Type,
						VolumeSize: machineType.Storage.StorageSize.String(),
					}
				}
			}
		}

		workers = append(workers, *workerCopy)
	}

	return workers
}

func lifetimeVerificationNeeded(new, old core.Shoot) bool {
	oldLifetime, ok := old.Annotations[v1beta1constants.ShootExpirationTimestamp]
	if !ok {
		oldLifetime = old.CreationTimestamp.String()
	}
	newLifetime := new.Annotations[v1beta1constants.ShootExpirationTimestamp]
	return oldLifetime != newLifetime
}

func quotaVerificationNeeded(new, old core.Shoot) bool {
	if !helper.NginxIngressEnabled(old.Spec.Addons) && helper.NginxIngressEnabled(new.Spec.Addons) {
		return true
	}

	// Check for diffs on workers
	for _, worker := range new.Spec.Provider.Workers {
		oldHasWorker := false
		for _, oldWorker := range old.Spec.Provider.Workers {
			if worker.Name == oldWorker.Name {
				oldHasWorker = true
				if worker.Machine.Type != oldWorker.Machine.Type || worker.Maximum != oldWorker.Maximum || !apiequality.Semantic.DeepEqual(worker.Volume, oldWorker.Volume) {
					return true
				}
			}
		}
		if !oldHasWorker {
			return true
		}
	}

	return false
}

func hasSufficientQuota(limit, required resource.Quantity) bool {
	compareCode := limit.Cmp(required)
	return compareCode != -1
}

func sumQuantity(values ...resource.Quantity) resource.Quantity {
	res := resource.Quantity{}
	for _, v := range values {
		res.Add(v)
	}
	return res
}

func multiplyQuantity(quantity resource.Quantity, multiplier int32) resource.Quantity {
	res := resource.Quantity{}
	for i := 0; i < int(multiplier); i++ {
		res.Add(quantity)
	}
	return res
}
