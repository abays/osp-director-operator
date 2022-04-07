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

package v1beta2

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	goClient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var openstackbackuprequestlog = logf.Log.WithName("openstackbackuprequest-resource")

// Client needed for API calls (manager's client, set by first SetupWebhookWithManager() call
// to any particular webhook)
var webhookClient goClient.Client

// SetupWebhookWithManager -
func (r *OpenStackBackupRequest) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhookClient == nil {
		webhookClient = mgr.GetClient()
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-osp-director-openstack-org-v1beta2-openstackbackuprequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=osp-director.openstack.org,resources=openstackbackuprequests,verbs=create;update,versions=v1beta2,name=vopenstackbackuprequest2.kb.io,admissionReviewVersions={v1},sideEffects=None

var _ webhook.Validator = &OpenStackBackupRequest{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackBackupRequest) ValidateCreate() error {
	openstackbackuprequestlog.Info("validate create", "name", r.Name)

	if err := r.validateCr(); err != nil {
		return err
	}

	currentBackupOperation, err := GetOpenStackBackupOperationInProgress(webhookClient, r.Namespace)

	if err != nil {
		return err
	}

	if currentBackupOperation != "" {
		return fmt.Errorf("cannot create a new backup request while an existing backup request is %s", currentBackupOperation)
	}

	return r.validateCr()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackBackupRequest) ValidateUpdate(old runtime.Object) error {
	openstackbackuprequestlog.Info("validate update", "name", r.Name)

	return r.validateCr()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackBackupRequest) ValidateDelete() error {
	openstackbackuprequestlog.Info("validate delete", "name", r.Name)

	return nil
}

func (r *OpenStackBackupRequest) validateCr() error {
	// Removed because it causes an import cycle given the conversion logic in api/v1beta1 and here in api/v1beta1 -- not important
	// for this example anyhow
	//
	// if r.Spec.BackupMode == BackupRestore || r.Spec.BackupMode == BackupCleanRestore {
	// 	return webhookClient.Get(context.TODO(), types.NamespacedName{Name: r.Spec.RestoreSource, Namespace: r.Namespace}, &ospdirectorv1beta1.OpenStackBackup{})
	// }

	return nil
}
