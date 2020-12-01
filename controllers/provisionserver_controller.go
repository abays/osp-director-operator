/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metal3valpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	machinev1beta1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	ospdirectorv1beta1 "github.com/openstack-k8s-operators/osp-director-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/osp-director-operator/pkg/common"
	provisionserver "github.com/openstack-k8s-operators/osp-director-operator/pkg/provisionserver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ProvisionServerReconciler reconciles a ProvisionServer object
type ProvisionServerReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *ProvisionServerReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *ProvisionServerReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *ProvisionServerReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *ProvisionServerReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// +kubebuilder:rbac:groups=osp-director.openstack.org,resources=provisionservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=osp-director.openstack.org,resources=provisionservers/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=osp-director.openstack.org,resources=provisionservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete;watch;
// +kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=get;list;create;update;delete;watch;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;delete;watch;
// +kubebuilder:rbac:groups=core,resources=volumes,verbs=get;list;create;update;delete;watch;
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;update;watch;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;update;watch;
// +kubebuilder:rbac:groups=machine.openshift.io,resources="*",verbs="*"
// +kubebuilder:rbac:groups=metal3.io,resources="*",verbs="*"
// +kubebuilder:rbac:groups=security.openshift.io,namespace=openstack,resources="securitycontextconstraints",resourceNames="privileged",verbs="use"
// +kubebuilder:rbac:groups=security.openshift.io,namespace=openstack,resources="securitycontextconstraints",resourceNames="anyuid",verbs="use"

// Reconcile - provision image servers
func (r *ProvisionServerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("provisionserver", req.NamespacedName)

	// Fetch the ProvisionServer instance
	instance := &ospdirectorv1beta1.ProvisionServer{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// config maps
	envVars := make(map[string]common.EnvSetter)
	cmLabels := common.GetLabels(instance.Name, provisionserver.AppLabel)

	templateParameters := make(map[string]string)
	templateParameters["Port"] = strconv.Itoa(instance.Spec.Port)

	cm := []common.Template{
		// Apache server config
		{
			Name:           fmt.Sprintf("%s-httpd-config", instance.Name),
			Namespace:      instance.Namespace,
			Type:           common.TemplateTypeConfig,
			InstanceType:   instance.Kind,
			AdditionalData: map[string]string{},
			Labels:         cmLabels,
			ConfigOptions:  templateParameters,
		},
	}

	err = common.EnsureConfigMaps(r, instance, cm, &envVars)

	if err != nil {
		return ctrl.Result{}, nil
	}

	// provisionserver
	// Create or update the Deployment object
	op, err := r.deploymentCreateOrUpdate(instance)

	if err != nil {
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}

	// Get the provisioning IP of node hosting the pod belonging to the CR
	podIP, err := r.getProvisionServerProvisioningIP(instance)

	if err != nil {
		return ctrl.Result{}, err
	}

	if podIP == "" {
		r.Log.Info(fmt.Sprintf("Deployment %s pod provisioning IP not yet available, requeuing and waiting", instance.Name))
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Update Status
	instance.Status.LocalImageURL = r.getLocalImageURL(podIP, instance)
	err = r.Client.Status().Update(context.TODO(), instance)

	if err != nil {
		r.Log.Error(err, "Failed to update CR status %v")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager - prepare controller for use with operator manager
func (r *ProvisionServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// TODO: Myabe use filtering functions here since some resource permissions
	// are now cluster-scoped?
	return ctrl.NewControllerManagedBy(mgr).
		For(&ospdirectorv1beta1.ProvisionServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

func (r *ProvisionServerReconciler) deploymentCreateOrUpdate(instance *ospdirectorv1beta1.ProvisionServer) (controllerutil.OperationResult, error) {

	// Get volumes
	initVolumeMounts := provisionserver.GetInitVolumeMounts(instance.Name)
	volumeMounts := provisionserver.GetVolumeMounts(instance.Name)
	volumes := provisionserver.GetVolumes(instance.Name)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"deployment": instance.Name + "-provisionserver-deployment"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"deployment": instance.Name + "-provisionserver-deployment"},
				},
			},
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {

		replicas := int32(1)

		deployment.Spec.Replicas = &replicas
		deployment.Spec.Template.Spec = corev1.PodSpec{
			HostNetwork: true,
			Volumes:     volumes,
			Containers: []corev1.Container{
				{
					Name:         "osp-httpd",
					Image:        "quay.io/abays/httpd:2.4-alpine",
					Env:          []corev1.EnvVar{},
					VolumeMounts: volumeMounts,
				},
			},
		}

		initContainerDetails := provisionserver.InitContainer{
			ContainerImage: "quay.io/abays/downloader:latest",
			RhelImageURL:   instance.Spec.RhelImageURL,
			Privileged:     false,
			VolumeMounts:   initVolumeMounts,
		}

		deployment.Spec.Template.Spec.InitContainers = provisionserver.GetInitContainer(initContainerDetails)

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)

		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}

func (r *ProvisionServerReconciler) getProvisionServerProvisioningIP(instance *ospdirectorv1beta1.ProvisionServer) (string, error) {
	// Get the pod associated with the deployment
	labelSelectorString := labels.Set(map[string]string{
		"deployment": instance.Name + "-provisionserver-deployment",
	}).String()

	podList, err := r.Kclient.CoreV1().Pods(instance.Namespace).List(
		context.TODO(),
		metav1.ListOptions{
			LabelSelector: labelSelectorString,
		},
	)

	if err != nil {
		return "", err
	}

	if len(podList.Items) < 1 {
		return "", nil
	} else if len(podList.Items) > 1 {
		err := fmt.Errorf("Expected 1 pod for %s deployment, got %d", instance.Name, len(podList.Items))
		r.Log.Error(err, "Invalid pod count")
		return "", err
	}

	pod := podList.Items[0]
	r.Log.Info(fmt.Sprintf("Found pod %s on node %s", pod.Name, pod.Spec.NodeName))

	// If the pod is not running yet, we don't actually have the server available
	// on the provisioning IP, so just stop here for now
	if pod.Status.Phase != corev1.PodRunning {
		return "", nil
	}

	// Get node on which pod is scheduled
	node, err := r.Kclient.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})

	if err != nil {
		return "", err
	}

	machineName := strings.Split(node.ObjectMeta.Annotations["machine.openshift.io/machine"], "/")[1]
	r.Log.Info(fmt.Sprintf("Found node %s on machine %s", node.Name, machineName))

	// Get machine associated with node
	machine := &machinev1beta1.Machine{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: machineName, Namespace: "openshift-machine-api"}, machine)

	if err != nil {
		return "", err
	}

	bmhName := strings.Split(machine.ObjectMeta.Annotations["metal3.io/BareMetalHost"], "/")[1]
	r.Log.Info(fmt.Sprintf("Found machine %s on bare metal host %s", machine.Name, bmhName))

	// Get baremetalhost associated with machine
	bmh := &metal3valpha1.BareMetalHost{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bmhName, Namespace: "openshift-machine-api"}, bmh)

	if err != nil {
		return "", err
	}

	r.Log.Info(fmt.Sprintf("Found baremetalhost %s", bmh.Name))

	// TODO: There is likely a better way we should be doing this.
	// Get baremetalhost's provisioning IP by using its spec's "bootMACAddress" and finding
	// the nic in its status with that MAC address
	provMAC := bmh.Spec.BootMACAddress
	provIP := ""

	if bmh.Status.HardwareDetails != nil {
		for _, nic := range bmh.Status.HardwareDetails.NIC {
			if nic.MAC == provMAC {
				provIP = nic.IP
				r.Log.Info(fmt.Sprintf("Found provisioning IP %s on baremetalhost %s", provIP, bmh.Name))
				break
			}
		}
	}

	return provIP, nil
}

func (r *ProvisionServerReconciler) getLocalImageURL(podIP string, instance *ospdirectorv1beta1.ProvisionServer) string {
	baseFilename := instance.Spec.RhelImageURL[strings.LastIndex(instance.Spec.RhelImageURL, "/")+1 : len(instance.Spec.RhelImageURL)]
	baseFilenameEnd := baseFilename[len(baseFilename)-3 : len(baseFilename)]

	if baseFilenameEnd == ".gz" || baseFilenameEnd == ".xz" {
		baseFilename = baseFilename[0 : len(baseFilename)-3]
	}

	return fmt.Sprintf("http://%s:%d/images/%s/compressed-%s", podIP, instance.Spec.Port, baseFilename, baseFilename)
}
