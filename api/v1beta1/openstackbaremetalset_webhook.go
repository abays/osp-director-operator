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

// Generated by:
//
// operator-sdk create webhook --group osp-director --version v1beta1 --kind OpenStackBaremetalSet --programmatic-validation
//

package v1beta1

import (
	"context"
	"fmt"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/openstack-k8s-operators/osp-director-operator/api/shared"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var baremetalsetlog = logf.Log.WithName("baremetalset-resource")

// SetupWebhookWithManager - register this webhook with the controller manager
func (r *OpenStackBaremetalSet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhookClient == nil {
		webhookClient = mgr.GetClient()
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-osp-director-openstack-org-v1beta1-openstackbaremetalset,mutating=false,failurePolicy=fail,sideEffects=None,groups=osp-director.openstack.org,resources=openstackbaremetalsets,versions=v1beta1,name=vopenstackbaremetalset.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &OpenStackBaremetalSet{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackBaremetalSet) ValidateCreate() error {
	baremetalsetlog.Info("validate create", "name", r.Name)

	if err := CheckBackupOperationBlocksAction(r.Namespace, shared.APIActionCreate); err != nil {
		return err
	}

	//
	// Fail early on create if osnetcfg ist not found
	//
	_, err := GetOsNetCfg(webhookClient, r.GetNamespace(), r.GetLabels()[shared.OpenStackNetConfigReconcileLabel])
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("error getting OpenStackNetConfig %s - %s: %s",
			r.GetLabels()[shared.OpenStackNetConfigReconcileLabel],
			r.Name,
			err))
	}

	//
	// validate that for all configured subnets an osnet exists
	//
	if err := ValidateNetworks(r.GetNamespace(), r.Spec.Networks); err != nil {
		return err
	}

	//
	// Validate that there are enough available BMHs for the initial requested count
	//
	baremetalHostsList, err := GetBmhHosts(
		context.TODO(),
		webhookClient,
		"openshift-machine-api",
		r.Spec.BmhLabelSelector,
	)
	if err != nil {
		return err
	}

	if _, err := VerifyBaremetalSetScaleUp(baremetalsetlog, r, baremetalHostsList, &metal3v1alpha1.BareMetalHostList{}); err != nil {
		return err
	}

	return r.validateCr()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackBaremetalSet) ValidateUpdate(old runtime.Object) error {
	baremetalsetlog.Info("validate update", "name", r.Name)

	//
	// validate that for all configured subnets an osnet exists
	//
	if err := ValidateNetworks(r.GetNamespace(), r.Spec.Networks); err != nil {
		return err
	}

	var ok bool
	var oldInstance *OpenStackBaremetalSet

	if oldInstance, ok = old.(*OpenStackBaremetalSet); !ok {
		return fmt.Errorf("runtime object is not an OpenStackBaremetalSet")
	}

	//
	// Validate that there are enough available BMHs for a potential scale-up or scale-down
	//
	if r.Spec.Count > oldInstance.Spec.Count {
		baremetalHostsList, err := GetBmhHosts(
			context.TODO(),
			webhookClient,
			"openshift-machine-api",
			r.Spec.BmhLabelSelector,
		)
		if err != nil {
			return err
		}

		existingBaremetalHosts, err := GetBmhHosts(
			context.TODO(),
			webhookClient,
			"openshift-machine-api",
			map[string]string{
				shared.OwnerControllerNameLabelSelector: shared.OpenStackBaremetalSetAppLabel,
				shared.OwnerUIDLabelSelector:            string(r.GetUID()),
			},
		)
		if err != nil {
			return err
		}

		if _, err := VerifyBaremetalSetScaleUp(baremetalsetlog, r, baremetalHostsList, existingBaremetalHosts); err != nil {
			return err
		}
	} else if r.Spec.Count < oldInstance.Spec.Count {
		existingBaremetalHosts, err := GetBmhHosts(
			context.TODO(),
			webhookClient,
			"openshift-machine-api",
			map[string]string{
				shared.OwnerControllerNameLabelSelector: shared.OpenStackBaremetalSetAppLabel,
				shared.OwnerUIDLabelSelector:            string(r.GetUID()),
			},
		)
		if err != nil {
			return err
		}

		annotatedBaremetalHosts := getDeletionAnnotatedBmhHosts(existingBaremetalHosts)

		if err := VerifyBaremetalSetScaleDown(baremetalsetlog, r, existingBaremetalHosts, len(annotatedBaremetalHosts)); err != nil {
			return err
		}
	}

	return r.validateCr()

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackBaremetalSet) ValidateDelete() error {
	baremetalsetlog.Info("validate delete", "name", r.Name)

	return CheckBackupOperationBlocksAction(r.Namespace, shared.APIActionDelete)
}

//+kubebuilder:webhook:path=/mutate-osp-director-openstack-org-v1beta1-openstackbaremetalset,mutating=true,failurePolicy=fail,sideEffects=None,groups=osp-director.openstack.org,resources=openstackbaremetalsets,verbs=create;update,versions=v1beta1,name=mopenstackbaremetalset.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &OpenStackBaremetalSet{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *OpenStackBaremetalSet) Default() {
	baremetalsetlog.Info("default", "name", r.Name)

	//
	// set OpenStackNetConfig reference label if not already there
	// Note, any rename of the osnetcfg won't be reflected
	//
	if _, ok := r.GetLabels()[shared.OpenStackNetConfigReconcileLabel]; !ok {
		labels, err := AddOSNetConfigRefLabel(
			webhookClient,
			r.Namespace,
			r.Spec.Networks[0],
			r.DeepCopy().GetLabels(),
		)
		if err != nil {
			baremetalsetlog.Error(err, fmt.Sprintf("error adding OpenStackNetConfig reference label on %s - %s: %s", r.Kind, r.Name, err))
		}

		r.SetLabels(labels)
		baremetalsetlog.Info(fmt.Sprintf("%s %s labels set to %v", r.GetObjectKind().GroupVersionKind().Kind, r.Name, r.GetLabels()))
	}

	//
	// add labels of all networks used by this CR
	//
	labels := AddOSNetNameLowerLabels(
		baremetalsetlog,
		r.DeepCopy().GetLabels(),
		r.Spec.Networks,
	)
	if !equality.Semantic.DeepEqual(
		labels,
		r.GetLabels(),
	) {
		r.SetLabels(labels)
		baremetalsetlog.Info(fmt.Sprintf("%s %s labels set to %v", r.GetObjectKind().GroupVersionKind().Kind, r.Name, r.GetLabels()))
	}

	//
	// set spec.domainName , dnsSearchDomains and dnsServers from osnetcfg if not specified
	//
	osNetCfg, err := GetOsNetCfg(webhookClient, r.GetNamespace(), r.GetLabels()[shared.OpenStackNetConfigReconcileLabel])
	if err != nil {
		baremetalsetlog.Error(err, fmt.Sprintf("error getting OpenStackNetConfig %s - %s: %s",
			r.GetLabels()[shared.OpenStackNetConfigReconcileLabel],
			r.Name,
			err))
	}

	if osNetCfg != nil {
		if len(r.Spec.DNSSearchDomains) == 0 && len(osNetCfg.Spec.DNSSearchDomains) > 0 {
			r.Spec.DNSSearchDomains = osNetCfg.Spec.DNSSearchDomains
			baremetalsetlog.Info(fmt.Sprintf("Using DNSSearchDomains from %s %s: %v", osNetCfg.GetObjectKind().GroupVersionKind().Kind, osNetCfg.Name, r.Spec.DNSSearchDomains))
		}
		if len(r.Spec.BootstrapDNS) == 0 && len(osNetCfg.Spec.DNSServers) > 0 {
			r.Spec.BootstrapDNS = osNetCfg.Spec.DNSServers
			baremetalsetlog.Info(fmt.Sprintf("Using BootstrapDNS from %s %s: %v", osNetCfg.GetObjectKind().GroupVersionKind().Kind, osNetCfg.Name, r.Spec.BootstrapDNS))
		}
	}

}

func (r *OpenStackBaremetalSet) validateCr() error {
	if err := r.checkBaseImageReqs(); err != nil {
		return err
	}

	if err := checkRoleNameExists(r.TypeMeta, r.ObjectMeta, r.Spec.RoleName); err != nil {
		return err
	}

	return nil
}
func (r *OpenStackBaremetalSet) checkBaseImageReqs() error {
	if r.Spec.BaseImageURL == "" && r.Spec.ProvisionServerName == "" {
		return fmt.Errorf("either \"baseImageUrl\" or \"provisionServerName\" must be provided")
	}

	return nil
}
