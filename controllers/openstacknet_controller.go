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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ospdirectorv1beta1 "github.com/openstack-k8s-operators/osp-director-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/osp-director-operator/pkg/common"
	openstacknet "github.com/openstack-k8s-operators/osp-director-operator/pkg/openstacknet"
	openstacknetattachment "github.com/openstack-k8s-operators/osp-director-operator/pkg/openstacknetattachment"
)

// OpenStackNetReconciler reconciles a OpenStackNet object
type OpenStackNetReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *OpenStackNetReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *OpenStackNetReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *OpenStackNetReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *OpenStackNetReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// +kubebuilder:rbac:groups=osp-director.openstack.org,resources=openstacknets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=osp-director.openstack.org,resources=openstacknets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=osp-director.openstack.org,resources=openstacknets/finalizers,verbs=update
// +kubebuilder:rbac:groups=osp-director.openstack.org,namespace=openstack,resources=deployments/finalizers,verbs=update
// FIXME: Cluster-scope required below for now, as the operator watches openshift-machine-api namespace as well
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=create;delete;deletecollection;get;list;patch;update;watch
// +kubebuilder:rbac:groups=nmstate.io,resources=nodenetworkconfigurationpolicies,verbs=create;delete;get;list;patch;update;watch

// Reconcile -
func (r *OpenStackNetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("openstacknet", req.NamespacedName)

	// Fetch the OpenStackNet instance
	instance := &ospdirectorv1beta1.OpenStackNet{}
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

	//
	// initialize condition
	//
	cond := &ospdirectorv1beta1.Condition{}

	//
	// If RoleReservations status map is nil, create it
	//
	if instance.Status.RoleReservations == nil {
		instance.Status.RoleReservations = map[string]ospdirectorv1beta1.OpenStackNetRoleStatus{}
	}

	//
	// Used in comparisons below to determine whether a status update is actually needed
	//
	currentStatus := instance.Status.DeepCopy()
	statusChanged := func() bool {
		return !equality.Semantic.DeepEqual(
			r.getNormalizedStatus(&instance.Status),
			r.getNormalizedStatus(currentStatus),
		)
	}

	defer func(cond *ospdirectorv1beta1.Condition) {
		//
		// Update object conditions
		//
		instance.Status.CurrentState = ospdirectorv1beta1.NetState(cond.Type)
		instance.Status.Conditions.UpdateCurrentCondition(
			cond.Type,
			ospdirectorv1beta1.ConditionReason(cond.Message),
			cond.Message,
		)

		if statusChanged() {
			if updateErr := r.Client.Status().Update(context.Background(), instance); updateErr != nil {
				if err == nil {
					err = common.WrapErrorForObject(
						"Update Status", instance, updateErr)
				} else {
					common.LogErrorForObject(r, updateErr, "Update status", instance)
				}
			}
		}
	}(cond)

	// examine DeletionTimestamp to determine if object is under deletion
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(instance, openstacknet.FinalizerName) {
			controllerutil.AddFinalizer(instance, openstacknet.FinalizerName)
			if err := r.Update(context.Background(), instance); err != nil {
				return ctrl.Result{}, err
			}
			common.LogForObject(r, fmt.Sprintf("Finalizer %s added to CR %s", openstacknet.FinalizerName, instance.Name), instance)
		}
	} else {
		//
		// 1. check if finalizer is there
		// Reconcile if finalizer got already removed
		if !controllerutil.ContainsFinalizer(instance, openstacknet.FinalizerName) {
			return ctrl.Result{}, nil
		}

		//
		// 2. Clean up resources used by the operator
		//
		// osnet resources
		err := r.cleanupNetworkAttachmentDefinition(instance)
		if err != nil {
			return ctrl.Result{}, err
		}

		//
		// 3. as last step remove the finalizer on the operator CR to finish delete
		//
		controllerutil.RemoveFinalizer(instance, openstacknet.FinalizerName)
		err = r.Client.Update(context.TODO(), instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		common.LogForObject(r, fmt.Sprintf("CR %s deleted", instance.Name), instance)

		return ctrl.Result{}, nil
	}

	// If we determine that a backup is overriding this reconcile, requeue after a longer delay
	overrideReconcile, err := ospdirectorv1beta1.OpenStackBackupOverridesReconcile(r.Client, instance.Namespace, instance.Status.CurrentState == ospdirectorv1beta1.NetConfigured)

	if err != nil {
		return ctrl.Result{}, err
	}

	if overrideReconcile {
		r.Log.Info(fmt.Sprintf("OpenStackNet %s reconcile overridden due to OpenStackBackupRequest(s) state; requeuing after 20 seconds", instance.Name))
		return ctrl.Result{RequeueAfter: time.Duration(20) * time.Second}, err
	}

	//
	// Create/update NetworkAttachmentDefinition
	//
	if err := r.createOrUpdateNetworkAttachmentDefinition(instance, false, cond); err != nil {
		cond.Message = fmt.Sprintf("OpenStackNet %s encountered an error configuring NetworkAttachmentDefinition", instance.Name)
		cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetError)
		return ctrl.Result{}, err
	}

	//
	// Create/update static NetworkAttachmentDefinition used for openstackclient
	//
	if err := r.createOrUpdateNetworkAttachmentDefinition(instance, true, cond); err != nil {
		cond.Message = fmt.Sprintf("OpenStackNet %s encountered an error configuring static NetworkAttachmentDefinition", instance.Name)
		cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetError)
		return ctrl.Result{}, err
	}

	//
	// Count IP assignments and update status
	//
	totalIps := 0

	for _, roleIps := range instance.Status.RoleReservations {
		totalIps += len(roleIps.Reservations)
	}

	instance.Status.ReservedIPCount = totalIps

	// If we get this far, we assume the NAD been successfully created (NAD does not
	// have a status block we can examine)
	cond.Message = fmt.Sprintf("OpenStackNet %s has been successfully configured on targeted node(s)", instance.Name)
	cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetConfigured)

	return ctrl.Result{}, nil
}

func (r *OpenStackNetReconciler) getNormalizedStatus(status *ospdirectorv1beta1.OpenStackNetStatus) *ospdirectorv1beta1.OpenStackNetStatus {

	//
	// set LastHeartbeatTime and LastTransitionTime to a default value as those
	// need to be ignored to compare if conditions changed.
	//
	s := status.DeepCopy()
	for _, cond := range s.Conditions {
		cond.LastHeartbeatTime = metav1.Time{}
		cond.LastTransitionTime = metav1.Time{}
	}

	return s
}

// SetupWithManager -
func (r *OpenStackNetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ospdirectorv1beta1.OpenStackNet{}).
		Owns(&networkv1.NetworkAttachmentDefinition{}).
		Complete(r)
}

//
// createOrUpdateNetworkAttachmentDefinition - create or update NetworkAttachmentDefinition
//
func (r *OpenStackNetReconciler) createOrUpdateNetworkAttachmentDefinition(
	instance *ospdirectorv1beta1.OpenStackNet,
	nadStatic bool,
	cond *ospdirectorv1beta1.Condition,
) error {
	networkAttachmentDefinition := &networkv1.NetworkAttachmentDefinition{}
	networkAttachmentDefinition.Name = instance.Name

	//
	// get bridge name from referenced osnetattach CR status
	//
	bridgeName, err := openstacknetattachment.GetOpenStackNetAttachmentBridgeName(
		r,
		instance.Namespace,
		instance.Spec.AttachConfiguration,
	)
	if err != nil {
		cond.Message = fmt.Sprintf("OpenStackNet %s failure get bridge for OpenStackNetAttachment %s", instance.Name, instance.Spec.AttachConfiguration)
		cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetError)

		return common.WrapErrorForObject(fmt.Sprintf("failure get bridge name for OpenStackNetAttachment referenc: %s", instance.Spec.AttachConfiguration), instance, err)
	}

	//
	// NAD static for openstackclient pods
	//
	templateData := map[string]string{
		"Name":       instance.Name,
		"BridgeName": bridgeName,
		"Vlan":       strconv.Itoa(instance.Spec.Vlan),
	}

	if nadStatic {
		networkAttachmentDefinition.Name = fmt.Sprintf("%s-static", instance.Name)
		templateData["Name"] = fmt.Sprintf("%s-static", instance.Name)
		templateData["Static"] = "true"
	}

	// render CNIConfigTemplate
	CNIConfig := common.ExecuteTemplateData(openstacknetattachment.CniConfigTemplate, templateData)
	if err := common.IsJSON(CNIConfig); err != nil {
		cond.Message = fmt.Sprintf("OpenStackNet %s failure rendering CNIConfig for NetworkAttachmentDefinition", instance.Name)
		cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetError)

		return common.WrapErrorForObject(fmt.Sprintf("failure rendering CNIConfig for NetworkAttachmentDefinition %s: %v", instance.Name, templateData), instance, err)
	}

	networkAttachmentDefinition.Namespace = instance.Namespace

	apply := func() error {
		common.InitMap(&networkAttachmentDefinition.Labels)
		common.InitMap(&networkAttachmentDefinition.Annotations)

		//
		// Labels
		//
		networkAttachmentDefinition.Labels[common.OwnerUIDLabelSelector] = string(instance.UID)
		networkAttachmentDefinition.Labels[common.OwnerNameLabelSelector] = instance.Name
		networkAttachmentDefinition.Labels[common.OwnerNameSpaceLabelSelector] = instance.Namespace
		networkAttachmentDefinition.Labels[common.OwnerControllerNameLabelSelector] = openstacknet.AppLabel

		//
		// Annotations
		//
		networkAttachmentDefinition.Annotations["k8s.v1.cni.cncf.io/resourceName"] = fmt.Sprintf("bridge.network.kubevirt.io/%s", templateData["BridgeName"])

		//
		// Spec
		//
		networkAttachmentDefinition.Spec = networkv1.NetworkAttachmentDefinitionSpec{
			Config: CNIConfig,
		}

		return controllerutil.SetControllerReference(instance, networkAttachmentDefinition, r.Scheme)
	}

	op, err := controllerutil.CreateOrUpdate(context.Background(), r.Client, networkAttachmentDefinition, apply)
	if err != nil {
		cond.Message = fmt.Sprintf("Updating %s networkAttachmentDefinition", instance.Name)
		cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetError)

		err = common.WrapErrorForObject(fmt.Sprintf("Updating %s networkAttachmentDefinition", instance.Name), networkAttachmentDefinition, err)
		return err
	}

	if op != controllerutil.OperationResultNone {
		common.LogForObject(r, string(op), networkAttachmentDefinition)

		cond.Message = fmt.Sprintf("NetworkAttachmentDefinition %s is %s", networkAttachmentDefinition.Name, string(op))
		cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetConfiguring)
	} else {
		cond.Message = fmt.Sprintf("NetworkAttachmentDefinition %s configured targeted node(s)", networkAttachmentDefinition.Name)
		cond.Type = ospdirectorv1beta1.ConditionType(ospdirectorv1beta1.NetConfigured)
	}

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(networkAttachmentDefinition, openstacknet.FinalizerName) {
			controllerutil.AddFinalizer(networkAttachmentDefinition, openstacknet.FinalizerName)
			if err := r.Update(context.Background(), networkAttachmentDefinition); err != nil {
				return err
			}
			common.LogForObject(r, fmt.Sprintf("Finalizer %s added to %s", openstacknet.FinalizerName, networkAttachmentDefinition.Name), instance)

		}
	}

	return nil
}

func (r *OpenStackNetReconciler) cleanupNetworkAttachmentDefinition(
	instance *ospdirectorv1beta1.OpenStackNet,
) error {

	networkAttachmentDefinitionList := &networkv1.NetworkAttachmentDefinitionList{}

	labelSelector := map[string]string{
		common.OwnerNameLabelSelector: instance.Name,
	}

	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(labelSelector),
	}

	if err := r.GetClient().List(context.Background(), networkAttachmentDefinitionList, listOpts...); err != nil {
		return err
	}

	//
	// Remove finalizer on NADs
	//
	for _, nad := range networkAttachmentDefinitionList.Items {
		controllerutil.RemoveFinalizer(&nad, openstacknet.FinalizerName)
		if err := r.Client.Update(context.TODO(), &nad); err != nil {
			return err
		}
	}

	//
	// Delete NADs
	//
	if err := r.GetClient().DeleteAllOf(
		context.TODO(),
		&networkv1.NetworkAttachmentDefinition{},
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string]string{
			common.OwnerNameLabelSelector: instance.Name,
		}),
	); err != nil && !k8s_errors.IsNotFound(err) {
		return common.WrapErrorForObject("error DeleteAllOf NetworkAttachmentDefinition", instance, err)
	}

	return nil
}
