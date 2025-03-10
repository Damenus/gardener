# Gardener Resource Manager

Initially, the gardener-resource-manager was a project similar to the [kube-addon-manager](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/addon-manager).
It manages Kubernetes resources in a target cluster which means that it creates, updates, and deletes them.
Also, it makes sure that manual modifications to these resources are reconciled back to the desired state.

In the Gardener project we were using the kube-addon-manager since more than two years.
While we have progressed with our [extensibility story](https://github.com/gardener/gardener/blob/master/docs/proposals/01-extensibility.md) (moving cloud providers out-of-tree) we had decided that the kube-addon-manager is no longer suitable for this use-case.
The problem with it is that it needs to have its managed resources on its file system.
This requires storing the resources in `ConfigMap`s or `Secret`s and mounting them to the kube-addon-manager pod during deployment time.
The gardener-resource-manager uses `CustomResourceDefinition`s which allows to dynamically add, change, and remove resources with immediate action and without the need to reconfigure the volume mounts/restarting the pod.

Meanwhile, the `gardener-resource-manager` has evolved to a more generic component comprising several controllers and webhook handlers.
It is deployed by gardenlet once per seed (in the `garden` namespace) and once per shoot (in the respective shoot namespaces in the seed).

## Controllers

### `ManagedResource` controller

This controller watches custom objects called `ManagedResource`s in the `resources.gardener.cloud/v1alpha1` API group.
These objects contain references to secrets which itself contain the resources to be managed.
The reason why a `Secret` is used to store the resources is that they could contain confidential information like credentials.

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: managedresource-example1
  namespace: default
type: Opaque
data:
  objects.yaml: YXBpVmVyc2lvbjogdjEKa2luZDogQ29uZmlnTWFwCm1ldGFkYXRhOgogIG5hbWU6IHRlc3QtMTIzNAogIG5hbWVzcGFjZTogZGVmYXVsdAotLS0KYXBpVmVyc2lvbjogdjEKa2luZDogQ29uZmlnTWFwCm1ldGFkYXRhOgogIG5hbWU6IHRlc3QtNTY3OAogIG5hbWVzcGFjZTogZGVmYXVsdAo=
    # apiVersion: v1
    # kind: ConfigMap
    # metadata:
    #   name: test-1234
    #   namespace: default
    # ---
    # apiVersion: v1
    # kind: ConfigMap
    # metadata:
    #   name: test-5678
    #   namespace: default
---
apiVersion: resources.gardener.cloud/v1alpha1
kind: ManagedResource
metadata:
  name: example
  namespace: default
spec:
  secretRefs:
  - name: managedresource-example1
```

In the above example, the controller creates two `ConfigMap`s in the `default` namespace.
When a user is manually modifying them they will be reconciled back to the desired state stored in the `managedresource-example` secret.

It is also possible to inject labels into all the resources:

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: managedresource-example2
  namespace: default
type: Opaque
data:
  other-objects.yaml: YXBpVmVyc2lvbjogYXBwcy92MSAjIGZvciB2ZXJzaW9ucyBiZWZvcmUgMS45LjAgdXNlIGFwcHMvdjFiZXRhMgpraW5kOiBEZXBsb3ltZW50Cm1ldGFkYXRhOgogIG5hbWU6IG5naW54LWRlcGxveW1lbnQKc3BlYzoKICBzZWxlY3RvcjoKICAgIG1hdGNoTGFiZWxzOgogICAgICBhcHA6IG5naW54CiAgcmVwbGljYXM6IDIgIyB0ZWxscyBkZXBsb3ltZW50IHRvIHJ1biAyIHBvZHMgbWF0Y2hpbmcgdGhlIHRlbXBsYXRlCiAgdGVtcGxhdGU6CiAgICBtZXRhZGF0YToKICAgICAgbGFiZWxzOgogICAgICAgIGFwcDogbmdpbngKICAgIHNwZWM6CiAgICAgIGNvbnRhaW5lcnM6CiAgICAgIC0gbmFtZTogbmdpbngKICAgICAgICBpbWFnZTogbmdpbng6MS43LjkKICAgICAgICBwb3J0czoKICAgICAgICAtIGNvbnRhaW5lclBvcnQ6IDgwCg==
    # apiVersion: apps/v1
    # kind: Deployment
    # metadata:
    #   name: nginx-deployment
    # spec:
    #   selector:
    #     matchLabels:
    #       app: nginx
    #   replicas: 2 # tells deployment to run 2 pods matching the template
    #   template:
    #     metadata:
    #       labels:
    #         app: nginx
    #     spec:
    #       containers:
    #       - name: nginx
    #         image: nginx:1.7.9
    #         ports:
    #         - containerPort: 80

---
apiVersion: resources.gardener.cloud/v1alpha1
kind: ManagedResource
metadata:
  name: example
  namespace: default
spec:
  secretRefs:
  - name: managedresource-example2
  injectLabels:
    foo: bar
```

In this example the label `foo=bar` will be injected into the `Deployment` as well as into all created `ReplicaSet`s and `Pod`s.

#### Preventing Reconciliations

If a ManagedResource is annotated with `resources.gardener.cloud/ignore=true` then it will be skipped entirely by the controller (no reconciliations or deletions of managed resources at all).
However, when the ManagedResource itself is deleted (for example when a shoot is deleted) then the annotation is not respected and all resources will be deleted as usual.
This feature can be helpful to temporarily patch/change resources managed as part of such ManagedResource.
Condition checks will be skipped for such ManagedResources.

#### Modes

The gardener-resource-manager can manage a resource in different modes. The supported modes are:
- `Ignore`
    - The corresponding resource is removed from the ManagedResource status (`.status.resources`). No action is performed on the cluster - the resource is no longer "managed" (updated or deleted).
    - The primary use case is a migration of a resource from one ManagedResource to another one.

The mode for a resource can be specified with the `resources.gardener.cloud/mode` annotation. The annotation should be specified in the encoded resource manifest in the Secret that is referenced by the ManagedResource.

#### Skipping health check

If a resource in the ManagedResource is annotated with `resources.gardener.cloud/skip-health-check=true` then the resource will be skipped during health checks by the health controller. The ManagedResource conditions will not reflect the health condition of this resource anymore. The `ResourcesProgressing` condition will also be set to `False`.

#### Resource Class

By default, gardener-resource-manager controller watches for ManagedResources in all namespaces. `--namespace` flag can be specified to gardener-resource-manager binary to restrict the watch to ManagedResources in a single namespace.
A ManagedResource has an optional `.spec.class` field that allows to indicate that it belongs to given class of resources. `--resource-class` flag can be specified to gardener-resource-manager binary to restrict the watch to ManagedResources with the given `.spec.class`. A default class is assumed if no class is specified.

#### Conditions

A ManagedResource has a ManagedResourceStatus, which has an array of Conditions. Conditions currently include:

| Condition              | Description                                               |
|------------------------|-----------------------------------------------------------|
| `ResourcesApplied`     | `True` if all resources are applied to the target cluster |
| `ResourcesHealthy`     | `True` if all resources are present and healthy           |
| `ResourcesProgressing` | `False` if all resources have been fully rolled out       |

`ResourcesApplied` may be `False` when:
- the resource `apiVersion` is not known to the target cluster
- the resource spec is invalid (for example the label value does not match the required regex for it)
- ...

`ResourcesHealthy` may be `False` when:
- the resource is not found
- the resource is a Deployment and the Deployment does not have the minimum availability.
- ...

`ResourcesProgressing` may be `True` when:
- a `Deployment`, `StatefulSet` or `DaemonSet` has not been fully rolled out yet, i.e. not all replicas have been updated with the latest changes to `spec.template`.

Each Kubernetes resources has different notion for being healthy. For example, a Deployment is considered healthy if the controller observed its current revision and if the number of updated replicas is equal to the number of replicas.

The following `status.conditions` section describes a healthy ManagedResource:

```yaml
conditions:
- lastTransitionTime: "2022-05-03T10:55:39Z"
  lastUpdateTime: "2022-05-03T10:55:39Z"
  message: All resources are healthy.
  reason: ResourcesHealthy
  status: "True"
  type: ResourcesHealthy
- lastTransitionTime: "2022-05-03T10:55:36Z"
  lastUpdateTime: "2022-05-03T10:55:36Z"
  message: All resources have been fully rolled out.
  reason: ResourcesRolledOut
  status: "False"
  type: ResourcesProgressing
- lastTransitionTime: "2022-05-03T10:55:18Z"
  lastUpdateTime: "2022-05-03T10:55:18Z"
  message: All resources are applied.
  reason: ApplySucceeded
  status: "True"
  type: ResourcesApplied
```

#### Ignoring Updates

In some cases it is not desirable to update or re-apply some of the cluster components (for example, if customization is required or needs to be applied by the end-user).
For these resources, the annotation "resources.gardener.cloud/ignore" needs to be set to "true" or a truthy value (Truthy values are "1", "t", "T", "true", "TRUE", "True") in the corresponding managed resource secrets,
this can be done from the components that create the managed resource secrets, for example Gardener extensions or Gardener. Once this is done, the resource will be initially created and later ignored during reconciliation.

#### Preserving `replicas` or `resources` in Workload Resources

The objects which are part of the `ManagedResource` can be annotated with

- `resources.gardener.cloud/preserve-replicas=true` in case the `.spec.replicas` field of workload resources like `Deployment`s, `StatefulSet`s, etc. shall be preserved during updates.
- `resources.gardener.cloud/preserve-resources=true` in case the `.spec.containers[*].resources` fields of all containers of workload resources like `Deployment`s, `StatefulSet`s, etc. shall be preserved during updates.

> This can be useful if there are non-standard horizontal/vertical auto-scaling mechanisms in place.
Standard mechanisms like `HorizontalPodAutoscaler` or `VerticalPodAutoscaler` will be auto-recognized by `gardener-resource-manager`, i.e., in such cases the annotations are not needed.

#### Origin

All the objects managed by the resource manager get a dedicated annotation
`resources.gardener.cloud/origin` describing the `ManagedResource` object that describes
this object. The default format is `<namespace>/<objectname>`.

In multi-cluster scenarios (the `ManagedResource` objects are maintained in a
cluster different from the one the described objects are managed), it might
be useful to include the cluster identity, as well.

This can be enforced by setting the `--cluster-id` option. Here, several
possibilities are supported:
- given a direct value: use this as id for the source cluster
- `<cluster>`: read the cluster identity from a `cluster-identity` config map
  in the `kube-system` namespace (attribute `cluster-identity`). This is
  automatically maintained in all clusters managed or involved in a gardener landscape.
- `<default>`: try to read the cluster identity from the config map. If not found,
  no identity is used
- empty string: no cluster identity is used (completely cluster local scenarios)

By default cluster id is not used. If cluster id is specified the format is `<cluster id>:<namespace>/<objectname>`.

In addition to the origin annotation, all objects managed by the resource manager get a dedicated label `resources.gardener.cloud/managed-by`. This label can be used to describe these objects with a [selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/). By default it is set to "gardener", but this can be overwritten by setting the `--managed-by-label` option.

### Garbage Collector For Immutable `ConfigMap`s/`Secret`s

In Kubernetes, workload resources (e.g., `Pod`s) can mount `ConfigMap`s or `Secret`s or reference them via environment variables in containers.
Typically, when the content of such `ConfigMap`/`Secret` gets changed then the respective workload is usually not dynamically reloading the configuration, i.e., a restart is required.
The most commonly used approach is probably having so-called [checksum annotations in the pod template](https://helm.sh/docs/howto/charts_tips_and_tricks/#automatically-roll-deployments) which makes Kubernetes to recreate the pod if the checksum changes.
However, it has the downside that old, still running versions of the workload might not be able to properly work with the already updated content in the `ConfigMap`/`Secret`, potentially causing application outages.

In order to protect users from such outages (and to also improve the performance of the cluster), the Kubernetes community provides the ["immutable `ConfigMap`s/`Secret`s feature"](https://kubernetes.io/docs/concepts/configuration/configmap/#configmap-immutable).
Enabling immutability requires `ConfigMap`s/`Secret`s to have unique names.
Having unique names requires the client to delete `ConfigMap`s`/`Secret`s no longer in use.

In order to provide a similarly lightweight experience for clients (compared to the well-established checksum annotation approach), the Gardener Resource Manager features an optional garbage collector controller (disabled by default).
The purpose of this controller is cleaning up such immutable `ConfigMap`s/`Secret`s if they are no longer in use.

#### How does the garbage collector work?

The following algorithm is implemented in the GC controller:

1. List all `ConfigMap`s and `Secret`s labeled with `resources.gardener.cloud/garbage-collectable-reference=true`.
1. List all `Deployment`s, `StatefulSet`s, `DaemonSet`s, `Job`s, `CronJob`s, `Pod`s and for each of them
    1. iterate over the `.metadata.annotations` and for each of them
        1. If the annotation key follows the `reference.resources.gardener.cloud/{configmap,secret}-<hash>` scheme and the value equals `<name>` then consider it as "in-use".
1. Delete all `ConfigMap`s and `Secret`s not considered as "in-use".

Consequently, clients need to

1. Create immutable `ConfigMap`s/`Secret`s with unique names (e.g., a checksum suffix based on the `.data`).
1. Label such `ConfigMap`s/`Secret`s with `resources.gardener.cloud/garbage-collectable-reference=true`.
1. Annotate their workload resources with `reference.resources.gardener.cloud/{configmap,secret}-<hash>=<name>` for all `ConfigMap`s/`Secret`s used by the containers of the respective `Pod`s.

   ⚠️ Add such annotations to `.metadata.annotations` as well as to all templates of other resources (e.g., `.spec.template.metadata.annotations` in `Deployment`s or `.spec.jobTemplate.metadata.annotations` and `.spec.jobTemplate.spec.template.metadata.annotations` for `CronJob`s.
   This ensures that the GC controller does not unintentionally consider `ConfigMap`s/`Secret`s as "not in use" just because there isn't a `Pod` referencing them anymore (e.g., they could still be used by a `Deployment` scaled down to `0`).

ℹ️ For the last step, there is a helper function `InjectAnnotations` in the `pkg/controller/garbagecollector/references` which you can use for your convenience.

**Example:**

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-1234
  namespace: default
  labels:
    resources.gardener.cloud/garbage-collectable-reference: "true"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-5678
  namespace: default
  labels:
    resources.gardener.cloud/garbage-collectable-reference: "true"
---
apiVersion: v1
kind: Pod
metadata:
  name: example
  namespace: default
  annotations:
    reference.resources.gardener.cloud/configmap-82a3537f: test-5678
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
    terminationGracePeriodSeconds: 2
```

The GC controller would delete the `ConfigMap/test-1234` because it is considered as not "in-use".

ℹ️ If the GC controller is activated then the `ManagedResource` controller will no longer delete `ConfigMap`s/`Secret`s having the above label.

#### How to activate the garbage collector?

The GC controller can be activated by providing the `--garbage-collector-sync-period` flag with a value larger than `0` (e.g., `1h`) to the Gardener Resource Manager.

### TokenInvalidator

The Kubernetes community is slowly transitioning from static `ServiceAccount` token `Secret`s to [`ServiceAccount` Token Volume Projection](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#service-account-token-volume-projection).
Typically, when you create a `ServiceAccount`

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: default
```

then the [`serviceaccount-token`](https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/serviceaccount/tokens_controller.go) controller (part of `kube-controller-manager`) auto-generates a `Secret` with a static token:

```yaml
apiVersion: v1
kind: Secret
metadata:
   annotations:
      kubernetes.io/service-account.name: default
      kubernetes.io/service-account.uid: 86e98645-2e05-11e9-863a-b2d4d086dd5a)
   name: default-token-ntxs9
type: kubernetes.io/service-account-token
data:
   ca.crt: base64(cluster-ca-cert)
   namespace: base64(namespace)
   token: base64(static-jwt-token)
```

Unfortunately, when using `ServiceAccount` Token Volume Projection in a `Pod`, this static token is actually not used at all:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  serviceAccountName: default
  containers:
  - image: nginx
    name: nginx
    volumeMounts:
    - mountPath: /var/run/secrets/tokens
      name: token
  volumes:
  - name: token
    projected:
      sources:
      - serviceAccountToken:
          path: token
          expirationSeconds: 7200
```

While the `Pod` is now using an expiring and auto-rotated token, the static token is still generated and valid.

As of Kubernetes v1.22, there is neither a way of preventing `kube-controller-manager` to generate such static tokens, nor a way to proactively remove or invalidate them:

- https://github.com/kubernetes/kubernetes/issues/77599
- https://github.com/kubernetes/kubernetes/issues/77600

Disabling the `serviceaccount-token` controller is an option, however, especially in the Gardener context it may either break end-users or it may not even be possible to control such settings.
Also, even if a future Kubernetes version supports native configuration of above behaviour, Gardener still supports older versions which won't get such features but need a solution as well.

This is where the _TokenInvalidator_ comes into play:
Since it is not possible to prevent `kube-controller-manager` from generating static `ServiceAccount` `Secret`s, the _TokenInvalidator_ is - as its name suggests - just invalidating these tokens.
It considers all such `Secret`s belonging to `ServiceAccount`s with `.automountServiceAccountToken=false`.
By default, all namespaces in the target cluster are watched, however, this can be configured by specifying the `--target-namespace` flag.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-serviceaccount
automountServiceAccountToken: false
```

This will result in a static `ServiceAccount` token secret whose `token` value is invalid:

```yaml
apiVersion: v1
kind: Secret
metadata:
  annotations:
    kubernetes.io/service-account.name: my-serviceaccount
    kubernetes.io/service-account.uid: 86e98645-2e05-11e9-863a-b2d4d086dd5a
  name: my-serviceaccount-token-ntxs9
type: kubernetes.io/service-account-token
data:
  ca.crt: base64(cluster-ca-cert)
  namespace: base64(namespace)
  token: AAAA
```

Any attempt to regenerate the token or creating a new such secret will again make the component invalidating it.

> You can opt-out of this behaviour for `ServiceAccount`s setting `.automountServiceAccountToken=false` by labeling them with `token-invalidator.resources.gardener.cloud/skip=true`.

In order to enable the _TokenInvalidator_ you have to set `--token-invalidator-max-concurrent-workers` to a value larger than `0`.

Below graphic shows an overview of the Token Invalidator for Service account secrets in the Shoot cluster.
![image](images/resource-manager-token-invalidator.jpg)

### TokenRequestor

This controller provides the service to create and auto-renew tokens via the [`TokenRequest` API](https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-request-v1/).

It provides a functionality similar to the kubelet's [Service Account Token Volume Projection](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#service-account-token-volume-projection).
It was created to handle the special case of issuing tokens to pods that run in a different cluster than the API server they communicate with (hence, using the native token volume projection feature is not possible).

The controller differentiates between `source cluster` and `target cluster`.
The `source cluster` hosts the gardener-resource-manager pod. Secrets in this cluster are watched and modified by the controller.
The `target cluster` _can_ be configured to point to another cluster. The existence of ServiceAccounts are ensured and token requests are issued against the target.
When the gardener-resource-manager is deployed next to the Shoot's controlplane in the Seed the `source cluster` is the Seed while the `target cluster` points to the Shoot.

#### Reconciliation Loop

This controller reconciles secrets in all namespaces in the source cluster with the label: `resources.gardener.cloud/purpose: token-requestor`.
See [here](../../example/resource-manager/30-secret-tokenrequestor.yaml) for an example of the secret.

The controller ensures a `ServiceAccount` exists in the target cluster as specified in the annotations of the `Secret` in the source cluster:

```yaml
serviceaccount.resources.gardener.cloud/name: <sa-name>
serviceaccount.resources.gardener.cloud/namespace: <sa-namespace>
```

The requested tokens will act with the privileges which are assigned to this `ServiceAccount`.

The controller will then request a token via the [`TokenRequest` API](https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-request-v1/) and populate it into the `.data.token` field to the `Secret` in the source cluster.

Alternatively, the client can provide a raw kubeconfig (in YAML or JSON format) via the `Secret`'s `.data.kubeconfig` field.
The controller will then populate the requested token in the kubeconfig for the user used in the `.current-context`.
For example, if `.data.kubeconfig` is

```yaml
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: AAAA
    server: some-server-url
  name: shoot--foo--bar
contexts:
- context:
    cluster: shoot--foo--bar
    user: shoot--foo--bar-token
  name: shoot--foo--bar
current-context: shoot--foo--bar
kind: Config
preferences: {}
users:
- name: shoot--foo--bar-token
  user:
    token: ""
```

then the `.users[0].user.token` field of the kubeconfig will be updated accordingly.

The controller also adds an annotation to the `Secret` to keep track when to renew the token before it expires.
By default, the tokens are issued to expire after 12 hours. The expiration time can be set with the following annotation:

```yaml
serviceaccount.resources.gardener.cloud/token-expiration-duration: 6h
```

It automatically renews once 80% of the lifetime is reached or after `24h`.

Optionally, the controller can also populate the token into a `Secret` in the target cluster. This can be requested by annotating the `Secret` in the source cluster with

```yaml
token-requestor.resources.gardener.cloud/target-secret-name: "foo"
token-requestor.resources.gardener.cloud/target-secret-namespace: "bar"
```

Overall, the TokenRequestor controller provides credentials with limited lifetime (JWT tokens) used by Shoot control plane components running in the Seed
to talk to the Shoot API Server.
Please see the graphic below:

![image](images/resource-manager-projected-token-controlplane-to-shoot-apiserver.jpg)

## Webhooks

### Auto-Mounting Projected `ServiceAccount` Tokens

When this webhook is activated then it automatically injects projected `ServiceAccount` token volumes into `Pod`s and all its containers if all of the following preconditions are fulfilled:

1. The `Pod` is NOT labeled with `projected-token-mount.resources.gardener.cloud/skip=true`.
2. The `Pod`'s `.spec.serviceAccountName` field is NOT empty and NOT set to `default`.
3. The `ServiceAccount` specified in the `Pod`'s `.spec.serviceAccountName` sets `.automountServiceAccountToken=false`.
4. The `Pod`'s `.spec.volumes[]` DO NOT already contain a volume with a name prefixed with `kube-api-access-`.

The projected volume will look as follows:

```yaml
spec:
  volumes:
  - name: kube-api-access-gardener
    projected:
      defaultMode: 420
      sources:
      - serviceAccountToken:
          expirationSeconds: 43200
          path: token
      - configMap:
          items:
          - key: ca.crt
            path: ca.crt
          name: kube-root-ca.crt
      - downwardAPI:
          items:
          - fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
            path: namespace
```

> The `expirationSeconds` are defaulted to `12h` and can be overwritten with the `--projected-token-mount-expiration-seconds` flag, or with the `projected-token-mount.resources.gardener.cloud/expiration-seconds` annotation on a `Pod` resource.

The volume will be mounted into all containers specified in the `Pod` to the path `/var/run/secrets/kubernetes.io/serviceaccount`.
This is the default location where client libraries expect to find the tokens and mimics the [upstream `ServiceAccount` admission plugin](https://github.com/kubernetes/kubernetes/tree/v1.22.2/plugin/pkg/admission/serviceaccount), see [this document](https://kubernetes.io/docs/reference/access-authn-authz/service-accounts-admin/#serviceaccount-admission-controller) for more information.

Overall, this webhook is used to inject projected service account tokens into pods running in the Shoot and the Seed cluster.
Hence, it is served from the Seed GRM and each Shoot GRM.
Please find an overview below for pods deployed in the Shoot cluster:

![image](images/resource-manager-projected-token-shoot-to-shoot-apiserver.jpg)

### Pod Zone Affinity

When this webhook is activated and namespaces are annotated with `control-plane.shoot.gardener.cloud/enforce-zone` then it automatically adds a [pod affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#inter-pod-affinity-and-anti-affinity) to all `Pod`s created in these namespaces:

```
spec:
  affinity:
    podAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector: {}
        topologyKey: topology.kubernetes.io/zone
```

In addition, if the annotation key `control-plane.shoot.gardener.cloud/enforce-zone` has a value `<zone-value>`, i.e. zone assigned, this information is added as part of a node affinity.

```
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: topology.kubernetes.io/zone
              operator: In
              values:
              - <zone-value>
```

Those terms let pods within a namespace being scheduled to nodes residing in the very same zone which is either randomly picked, or to a very specific zone.
In addition, the webhook removes any (anti-)affinities with `topology.kubernetes.io/zone` because they potentially contradict the above shown configuration.
Gardener uses this webhook to schedule control-plane pods within a single zone on a multi-zonal seed (seed with worker nodes across zones).
The goal is to reduce cross zonal network traffic within the seed with this approach.

