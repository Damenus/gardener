# `PriorityClasses` in Gardener Clusters

Gardener makes use of [`PriorityClasses`](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/) to improve overall robustness of the system.
In order to benefit from the full potential of `PriorityClasses`, gardenlet manages a set of well-known `PriorityClasses` with fine-granular priority values.

All components of the system should use these well-known `PriorityClasses` instead of creating and using separate ones with arbitrary values, which would compromise the overall goal of using `PriorityClasses` in the first place.
Gardenlet manages the well-known `PriorityClasses` listed in this document, so that third parties (e.g., Gardener extensions) can rely on them to be present when deploying components to Seed and Shoot clusters.

The listed well-known `PriorityClasses` follow this rough concept:

- Values are close to the maximum that can be declared by the user. This is important to ensure that Shoot system components have higher priority than the workload deployed by end-users.
- Values have a bit of headroom in between to ensure flexibility when the need for intermediate priority values arises.
- Values of `PriorityClasses` created on Seed clusters are lower than the ones on Shoots to ensure that Shoot system components have higher priority than Seed components, if the Seed is backed by a Shoot (`ManagedSeed`), e.g. `coredns` should have higher priority than `gardenlet`.
- Names simply include the last digits of the value to minimize confusion caused by many (similar) names like `critical`, `importance-high`, etc.


## `PriorityClasses` for Shoot System Components

| Name                                              | Priority   | Associated Components (Examples)                                                                                            |
|---------------------------------------------------|------------|-----------------------------------------------------------------------------------------------------------------------------|
| `system-node-critical` (created by Kubernetes)    | 2000001000 | `calico-node`, `kube-proxy`, `apiserver-proxy`, `csi-driver`, `egress-filter-applier`                                       |
| `system-cluster-critical` (created by Kubernetes) | 2000000000 | `calico-typha`, `calico-kube-controllers`, `coredns`, `vpn-shoot`                                                           |
| `gardener-shoot-system-900`                       | 999999900  | `node-problem-detector`                                                                                                     |
| `gardener-shoot-system-800`                       | 999999800  | `calico-typha-horizontal-autoscaler`, `calico-typha-vertical-autoscaler`                                                    |
| `gardener-shoot-system-700`                       | 999999700  | `blackbox-exporter`, `node-exporter`                                                                                        |
| `gardener-shoot-system-600`                       | 999999600  | `addons-nginx-ingress-controller`, `addons-nginx-ingress-k8s-backend`, `kubernetes-dashboard`, `kubernetes-metrics-scraper` |


## `PriorityClasses` for Seed System Components

| Name                               | Priority  | Associated Components (Examples)                                                                                                                                                                   |
|------------------------------------|-----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `gardener-system-critical`         | 999998950 | `gardenlet`, `gardener-resource-manager`, `istio-ingressgateway`, `istiod`                                                                                                                         |
| `gardener-system-900`              | 999998900 | Extensions, `gardener-seed-admission-controller`, `reversed-vpn-auth-server`                                                                                                                       |
| `gardener-system-800`              | 999998800 | `dependency-watchdog-endpoint`, `dependency-watchdog-probe`, `etcd-druid`, `(auditlog-)mutator`, `vpa-admission-controller`                                                                        |
| `gardener-system-700`              | 999998700 | `auditlog-seed-controller`, `hvpa-controller`, `vpa-recommender`, `vpa-updater`                                                                                                                    |
| `gardener-system-600`              | 999998600 | `aggregate-alertmanager`, `alertmanager`, `fluent-bit`, `grafana`, `kube-state-metrics`, `nginx-ingress-controller`, `nginx-k8s-backend`, `prometheus`, `loki`,  `seed-prometheus`, `vpa-exporter` |
| `gardener-reserve-excess-capacity` | -5        | `reserve-excess-capacity` ([ref](https://github.com/gardener/gardener/pull/6135))                                                                                                                  |

## `PriorityClasses` for Shoot Control Plane Components

| Name                  | Priority  | Associated Components (Examples)                                                                                                                                                      |
|-----------------------|-----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `gardener-system-500` | 999998500 | `etcd-events`, `etcd-main`, `kube-apiserver`                                                                                                                                          |
| `gardener-system-400` | 999998400 | `gardener-resource-manager`                                                                                                                                                           |
| `gardener-system-300` | 999998300 | `cloud-controller-manager`, `cluster-autoscaler`, `csi-driver-controller`, `kube-controller-manager`, `kube-scheduler`, `machine-controller-manager`, `terraformer, `vpn-seed-server` |
| `gardener-system-200` | 999998200 | `csi-snapshot-controller`, `csi-snapshot-validation`, `cert-controller-manager`, `shoot-dns-service`, `vpa-admission-controller`, `vpa-recommender`, `vpa-updater`                    |
| `gardener-system-100` | 999998100 | `alertmanager`, `grafana-operators`, `grafana-users`, `kube-state-metrics`, `prometheus`, `loki`, `event-logger`                                                                      |

There is also a legacy `PriorityClass` called `gardener-shoot-controlplane` with value `100`.
This `PriorityClass` is deprecated and will be removed in a future release.
Make sure to migrate all your components to the above listed fine-granular `PriorityClasses`.
