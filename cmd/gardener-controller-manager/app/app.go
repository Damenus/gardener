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

package app

import (
	"context"
	"fmt"
	"os"
	goruntime "runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/automaxprocs/maxprocs"
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
	"github.com/gardener/gardener/pkg/controllermanager/controller"
	controllermanagerfeatures "github.com/gardener/gardener/pkg/controllermanager/features"
	gardenerhealthz "github.com/gardener/gardener/pkg/healthz"
	"github.com/gardener/gardener/pkg/logger"
	"github.com/gardener/gardener/pkg/operation/garden"
	"github.com/gardener/gardener/pkg/server/routes"
)

// Name is a const for the name of this component.
const Name = "gardener-controller-manager"

// NewCommand creates a new cobra.Command for running gardener-controller-manager.
func NewCommand() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:   Name,
		Short: "Launch the " + Name,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			if err := opts.complete(); err != nil {
				return err
			}
			if err := opts.validate(); err != nil {
				return err
			}

			log, err := logger.NewZapLogger(opts.config.LogLevel, opts.config.LogFormat)
			if err != nil {
				return fmt.Errorf("error instantiating zap logger: %w", err)
			}

			logf.SetLogger(log)
			klog.SetLogger(log)

			log.Info("Starting "+Name, "version", version.Get())
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				log.Info(fmt.Sprintf("FLAG: --%s=%s", flag.Name, flag.Value)) //nolint:logcheck
			})

			// don't output usage on further errors raised during execution
			cmd.SilenceUsage = true
			// further errors will be logged properly, don't duplicate
			cmd.SilenceErrors = true

			return run(cmd.Context(), log, opts.config)
		},
	}

	flags := cmd.Flags()
	verflag.AddFlags(flags)
	opts.addFlags(flags)

	return cmd
}

func run(ctx context.Context, log logr.Logger, cfg *config.ControllerManagerConfiguration) error {
	// Add feature flags
	if err := controllermanagerfeatures.FeatureGate.SetFromMap(cfg.FeatureGates); err != nil {
		return err
	}
	log.Info("Feature Gates", "featureGates", controllermanagerfeatures.FeatureGate.String())

	// This is like importing the automaxprocs package for its init func (it will in turn call maxprocs.Set).
	// Here we pass a custom logger, so that the result of the library gets logged to the same logger we use for the
	// component itself.
	if _, err := maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		log.Info(fmt.Sprintf(s, i...)) //nolint:logcheck
	})); err != nil {
		log.Error(err, "Failed to set GOMAXPROCS")
	}

	log.Info("Getting rest config")
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		cfg.GardenClientConnection.Kubeconfig = kubeconfig
	}

	restConfig, err := kubernetes.RESTConfigFromClientConnectionConfiguration(&cfg.GardenClientConnection, nil, kubernetes.AuthTokenFile)
	if err != nil {
		return err
	}

	log.Info("Setting up manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Scheme:                  kubernetes.GardenScheme,
		HealthProbeBindAddress:  fmt.Sprintf("%s:%d", cfg.Server.HealthProbes.BindAddress, cfg.Server.HealthProbes.Port),
		MetricsBindAddress:      fmt.Sprintf("%s:%d", cfg.Server.Metrics.BindAddress, cfg.Server.Metrics.Port),
		GracefulShutdownTimeout: pointer.Duration(5 * time.Second),
		Logger:                  log,

		LeaderElection:             cfg.LeaderElection.LeaderElect,
		LeaderElectionResourceLock: cfg.LeaderElection.ResourceLock,
		LeaderElectionID:           cfg.LeaderElection.ResourceName,
		LeaderElectionNamespace:    cfg.LeaderElection.ResourceNamespace,
		LeaseDuration:              &cfg.LeaderElection.LeaseDuration.Duration,
		RenewDeadline:              &cfg.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:                &cfg.LeaderElection.RetryPeriod.Duration,

		// TODO: enable this once we have refactored all controllers and added them to this manager
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		return err
	}

	if cfg.Debugging != nil && cfg.Debugging.EnableProfiling {
		if err := (routes.Profiling{}).AddToManager(mgr); err != nil {
			return fmt.Errorf("failed adding profiling handlers to manager: %w", err)
		}
		if cfg.Debugging.EnableContentionProfiling {
			goruntime.SetBlockProfileRate(1)
		}
	}

	log.Info("Adding health check endpoints to manager")
	if err := mgr.AddReadyzCheck("informer-sync", gardenerhealthz.NewCacheSyncHealthz(mgr.GetCache())); err != nil {
		return err
	}
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}

	log.Info("Adding field indexes to informers")
	if err := controller.AddAllFieldIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return fmt.Errorf("failed adding indexes: %w", err)
	}

	log.Info("Adding garden bootstrapper to manager")
	if err := mgr.Add(&garden.Bootstrapper{
		Log:        log.WithName("bootstrap"),
		Client:     mgr.GetClient(),
		RESTConfig: restConfig,
	}); err != nil {
		return fmt.Errorf("failed adding garden cluster bootstrapper to manager: %w", err)
	}

	log.Info("Adding controllers to manager")
	if err := controller.AddControllersToManager(mgr, cfg); err != nil {
		return fmt.Errorf("failed adding controllers to manager: %w", err)
	}

	log.Info("Adding legacy controllers to manager")
	if err := mgr.Add(&controller.LegacyControllerFactory{
		Manager:    mgr,
		Log:        log,
		Config:     cfg,
		RESTConfig: restConfig,
	}); err != nil {
		return fmt.Errorf("failed adding legacy controllers to manager: %w", err)
	}

	log.Info("Starting manager")
	return mgr.Start(ctx)
}
