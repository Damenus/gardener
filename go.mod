module github.com/gardener/gardener

go 1.19

require (
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/ahmetb/gen-crd-api-reference-docs v0.2.0
	github.com/bronze1man/yaml2json v0.0.0-20211227013850-8972abeaea25
	github.com/coreos/go-systemd/v22 v22.3.2
	github.com/gardener/dependency-watchdog v0.7.0
	github.com/gardener/etcd-druid v0.12.3
	github.com/gardener/hvpa-controller/api v0.5.0
	github.com/gardener/machine-controller-manager v0.45.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v1.2.3
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.6.0
	github.com/google/gnostic v0.5.7-v3refs
	github.com/google/go-cmp v0.5.8
	github.com/hashicorp/go-multierror v1.1.1
	github.com/kubernetes-csi/external-snapshotter/v2 v2.1.4
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/mitchellh/hashstructure/v2 v2.0.2
	github.com/onsi/ginkgo/v2 v2.1.4
	github.com/onsi/gomega v1.20.0
	github.com/prometheus/client_golang v1.12.1
	github.com/robfig/cron v1.2.0
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.11.0
	github.com/texttheater/golang-levenshtein v1.0.1
	go.uber.org/automaxprocs v1.4.0
	go.uber.org/goleak v1.1.12
	go.uber.org/zap v1.21.0
	golang.org/x/crypto v0.0.0-20220516162934-403b01795ae8
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8
	golang.org/x/tools v0.1.12
	gomodules.xyz/jsonpatch/v2 v2.2.0
	gonum.org/v1/gonum v0.11.0
	google.golang.org/protobuf v1.28.0
	gopkg.in/yaml.v2 v2.4.0
	istio.io/api v0.0.0-20220512181135-e8ec1e1d89de
	istio.io/client-go v1.14.0
	k8s.io/api v0.24.4
	k8s.io/apiextensions-apiserver v0.24.4
	k8s.io/apimachinery v0.24.4
	k8s.io/apiserver v0.24.4
	k8s.io/autoscaler/vertical-pod-autoscaler v0.11.0
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/cluster-bootstrap v0.24.4
	k8s.io/code-generator v0.24.4
	k8s.io/component-base v0.24.4
	k8s.io/helm v2.16.1+incompatible
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.60.1
	k8s.io/kube-aggregator v0.24.4
	k8s.io/kube-openapi v0.0.0-20220328201542-3ee0da9b0b42
	k8s.io/kube-proxy v0.24.4
	k8s.io/kubelet v0.24.4
	k8s.io/metrics v0.24.4
	k8s.io/pod-security-admission v0.24.4
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	sigs.k8s.io/controller-runtime v0.12.1
	sigs.k8s.io/controller-runtime/tools/setup-envtest v0.0.0-20220613074012-11e533d55213 // v0.12.1
	sigs.k8s.io/controller-tools v0.9.0
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/frankban/quicktest v1.13.1 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-logr/zapr v1.2.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gobuffalo/flect v0.2.5 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/pprof v0.0.0-20210407192527-94a9f03dee38 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nwaples/rardecode v1.1.2 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/pelletier/go-toml/v2 v2.0.0-beta.8 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.etcd.io/etcd/api/v3 v3.5.2 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.2 // indirect
	go.etcd.io/etcd/client/v3 v3.5.1 // indirect
	go.opentelemetry.io/contrib v0.20.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.20.0 // indirect
	go.opentelemetry.io/otel v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/export/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/trace v0.20.0 // indirect
	go.opentelemetry.io/proto/otlp v0.7.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/oauth2 v0.0.0-20220411215720-9780585627b5 // indirect
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4 // indirect
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220407144326-9054f6ed7bac // indirect
	google.golang.org/grpc v1.45.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.66.4 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/gengo v0.0.0-20211129171323-c02415ce4185 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.30 // indirect
	sigs.k8s.io/json v0.0.0-20211208200746-9f7c6b3444d2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
)

replace (
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v2.9.5+incompatible // keep this value in sync with k8s.io/apiserver
	github.com/envoyproxy/go-control-plane => github.com/envoyproxy/go-control-plane v0.9.4
	github.com/gardener/hvpa-controller => github.com/gardener/hvpa-controller v0.5.0
	github.com/gardener/hvpa-controller/api => github.com/gardener/hvpa-controller/api v0.5.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.5.5 // keep this value in sync with k8s.io/apiserver
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.12.1 // keep this value in sync with sigs.k8s.io/controller-runtime
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20220516162934-403b01795ae8
	google.golang.org/grpc => google.golang.org/grpc v1.40.0 // keep this value in sync with k8s.io/apiserver
	k8s.io/api => k8s.io/api v0.24.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.24.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.24.4
	k8s.io/apiserver => k8s.io/apiserver v0.24.4
	k8s.io/autoscaler => k8s.io/autoscaler v0.0.0-20220531185024-cc90d57b7fe1 // translates to k8s.io/autoscaler/vertical-pod-autoscaler@v0.11.0
	k8s.io/client-go => k8s.io/client-go v0.24.4
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.4
	k8s.io/code-generator => k8s.io/code-generator v0.24.4
	k8s.io/component-base => k8s.io/component-base v0.24.4
	k8s.io/helm => k8s.io/helm v2.16.1+incompatible
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.4
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.4
)
