module github.com/abays/osp-director-operator

go 1.14

require (
	github.com/go-logr/logr v0.2.1
	github.com/metal3-io/baremetal-operator v0.0.0-20201107165446-c65c1ac8ddad
	github.com/onsi/ginkgo v1.13.0
	github.com/onsi/gomega v1.10.1
	github.com/openshift/cluster-api v0.0.0-20191129101638-b09907ac6668
	github.com/openstack-k8s-operators/lib-common v0.0.0-20201012132655-247b83b2fafa
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/common v0.10.0
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	k8s.io/api v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/controller-runtime v0.6.2
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.19.0
)