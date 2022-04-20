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

package v1beta1

import (
	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ospdirectorshared "github.com/openstack-k8s-operators/osp-director-operator/api/shared"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenStackVMSetSpec defines the desired state of an OpenStackVMSet
type OpenStackVMSetSpec struct {
	// Number of VMs to configure, 1 or 3
	VMCount int `json:"vmCount"`
	// number of Cores assigned to the VMs
	Cores uint32 `json:"cores"`
	// amount of Memory in GB used by the VMs
	Memory uint32 `json:"memory"`
	// root Disc size in GB
	DiskSize uint32 `json:"diskSize"`
	// StorageClass to be used for the disks
	StorageClass string `json:"storageClass,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ReadWriteMany
	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany
	// StorageAccessMode - Virtual machines must have a persistent volume claim (PVC)
	// with a shared ReadWriteMany (RWX) access mode to be live migrated.
	StorageAccessMode string `json:"storageAccessMode,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Filesystem
	// +kubebuilder:validation:Enum=Block;Filesystem
	// StorageVolumeMode - When using OpenShift Virtualization with OpenShift Container Platform Container Storage,
	// specify RBD block mode persistent volume claims (PVCs) when creating virtual machine disks. With virtual machine disks,
	// RBD block mode volumes are more efficient and provide better performance than Ceph FS or RBD filesystem-mode PVCs.
	// To specify RBD block mode PVCs, use the 'ocs-storagecluster-ceph-rbd' storage class and VolumeMode: Block.
	StorageVolumeMode string `json:"storageVolumeMode"`
	// BaseImageVolumeName used as the base volume for the VM
	BaseImageVolumeName string `json:"baseImageVolumeName"`
	// name of secret holding the stack-admin ssh keys
	DeploymentSSHSecret string `json:"deploymentSSHSecret"`

	// +kubebuilder:default=enp2s0
	// Interface to use for ctlplane network
	CtlplaneInterface string `json:"ctlplaneInterface"`

	// +kubebuilder:default={ctlplane,external,internalapi,tenant,storage,storagemgmt}
	// Networks the name(s) of the OpenStackNetworks used to generate IPs
	Networks []string `json:"networks"`

	// RoleName the name of the TripleO role this VM Spec is associated with. If it is a TripleO role, the name must match.
	RoleName string `json:"roleName"`
	// in case of external functionality, like 3rd party network controllers, set to false to ignore role in rendered overcloud templates.
	IsTripleoRole bool `json:"isTripleoRole"`
	// PasswordSecret the name of the secret used to optionally set the root pwd by adding
	// NodeRootPassword: <base64 enc pwd>
	// to the secret data
	PasswordSecret string `json:"passwordSecret,omitempty"`

	// BootstrapDNS - initial DNS nameserver values to set on the VM when they are provisioned.
	// Note that subsequent TripleO deployment will overwrite these values
	BootstrapDNS     []string `json:"bootstrapDns,omitempty"`
	DNSSearchDomains []string `json:"dnsSearchDomains,omitempty"`
}

// OpenStackVMSetStatus defines the observed state of OpenStackVMSet
type OpenStackVMSetStatus struct {
	// BaseImageDVReady is the status of the BaseImage DataVolume
	BaseImageDVReady   bool                             `json:"baseImageDVReady,omitempty"`
	Conditions         ospdirectorshared.ConditionList  `json:"conditions,omitempty" optional:"true"`
	ProvisioningStatus OpenStackVMSetProvisioningStatus `json:"provisioningStatus,omitempty"`
	// VMpods are the names of the kubevirt controller vm pods
	VMpods  []string              `json:"vmpods,omitempty"`
	VMHosts map[string]HostStatus `json:"vmHosts,omitempty"`
}

// OpenStackVMSetProvisioningStatus represents the overall provisioning state of all VMs in
// the OpenStackVMSet (with an optional explanatory message)
type OpenStackVMSetProvisioningStatus struct {
	State      ProvisioningState `json:"state,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	ReadyCount int               `json:"readyCount,omitempty"`
}

const (
	//
	// condition types
	//

	// VMSetCondTypeEmpty - special state for 0 requested VMs and 0 already provisioned
	VMSetCondTypeEmpty ProvisioningState = "Empty"
	// VMSetCondTypeWaiting - something is causing the OpenStackVmSet to wait
	VMSetCondTypeWaiting ProvisioningState = "Waiting"
	// VMSetCondTypeProvisioning - one or more VMs are provisioning
	VMSetCondTypeProvisioning ProvisioningState = "Provisioning"
	// VMSetCondTypeProvisioned - the requested VM count has been satisfied
	VMSetCondTypeProvisioned ProvisioningState = "Provisioned"
	// VMSetCondTypeDeprovisioning - one or more VMs are deprovisioning
	VMSetCondTypeDeprovisioning ProvisioningState = "Deprovisioning"
	// VMSetCondTypeError - general catch-all for actual errors
	VMSetCondTypeError ProvisioningState = "Error"

	//
	// condition reasones
	//

	// VMSetCondReasonError - error creating osvmset
	VMSetCondReasonError ospdirectorshared.ConditionReason = "OpenStackVMSetError"
	// VMSetCondReasonInitialize - vmset initialize
	VMSetCondReasonInitialize ospdirectorshared.ConditionReason = "OpenStackVMSetInitialize"
	// VMSetCondReasonProvisioning - vmset provisioning
	VMSetCondReasonProvisioning ospdirectorshared.ConditionReason = "OpenStackVMSetProvisioning"
	// VMSetCondReasonDeprovisioning - vmset deprovisioning
	VMSetCondReasonDeprovisioning ospdirectorshared.ConditionReason = "OpenStackVMSetDeprovisioning"
	// VMSetCondReasonProvisioned - vmset provisioned
	VMSetCondReasonProvisioned ospdirectorshared.ConditionReason = "OpenStackVMSetProvisioned"
	// VMSetCondReasonCreated - vmset created
	VMSetCondReasonCreated ospdirectorshared.ConditionReason = "OpenStackVMSetCreated"

	// VMSetCondReasonNamespaceFencingDataError - error creating the namespace fencing data
	VMSetCondReasonNamespaceFencingDataError ospdirectorshared.ConditionReason = "NamespaceFencingDataError"
	// VMSetCondReasonKubevirtFencingServiceAccountError - error creating/reading the KubevirtFencingServiceAccount secret
	VMSetCondReasonKubevirtFencingServiceAccountError ospdirectorshared.ConditionReason = "KubevirtFencingServiceAccountError"
	// VMSetCondReasonKubeConfigError - error getting the KubeConfig used by the operator
	VMSetCondReasonKubeConfigError ospdirectorshared.ConditionReason = "KubeConfigError"
	// VMSetCondReasonCloudInitSecretError - error creating the CloudInitSecret
	VMSetCondReasonCloudInitSecretError ospdirectorshared.ConditionReason = "CloudInitSecretError"
	// VMSetCondReasonDeploymentSecretMissing - deployment secret does not exist
	VMSetCondReasonDeploymentSecretMissing ospdirectorshared.ConditionReason = "DeploymentSecretMissing"
	// VMSetCondReasonDeploymentSecretError - deployment secret error
	VMSetCondReasonDeploymentSecretError ospdirectorshared.ConditionReason = "DeploymentSecretError"
	// VMSetCondReasonPasswordSecretMissing - password secret does not exist
	VMSetCondReasonPasswordSecretMissing ospdirectorshared.ConditionReason = "PasswordSecretMissing"
	// VMSetCondReasonPasswordSecretError - password secret error
	VMSetCondReasonPasswordSecretError ospdirectorshared.ConditionReason = "PasswordSecretError"

	// VMSetCondReasonVirtualMachineGetError - failed to get virtual machine
	VMSetCondReasonVirtualMachineGetError ospdirectorshared.ConditionReason = "VirtualMachineGetError"
	// VMSetCondReasonVirtualMachineAnnotationMissmatch - Unable to find sufficient amount of VirtualMachine replicas annotated for scale-down
	VMSetCondReasonVirtualMachineAnnotationMissmatch ospdirectorshared.ConditionReason = "VirtualMachineAnnotationMissmatch"
	// VMSetCondReasonVirtualMachineNetworkDataError - Error creating VM NetworkData
	VMSetCondReasonVirtualMachineNetworkDataError ospdirectorshared.ConditionReason = "VMSetCondReasonVirtualMachineNetworkDataError"
	// VMSetCondReasonVirtualMachineProvisioning - virtual machine provisioning in progress
	VMSetCondReasonVirtualMachineProvisioning ospdirectorshared.ConditionReason = "VirtualMachineProvisioning"
	// VMSetCondReasonVirtualMachineDeprovisioning - virtual machine deprovisioning in progress
	VMSetCondReasonVirtualMachineDeprovisioning ospdirectorshared.ConditionReason = "VirtualMachineDeprovisioning"
	// VMSetCondReasonVirtualMachineProvisioned - virtual machines provisioned
	VMSetCondReasonVirtualMachineProvisioned ospdirectorshared.ConditionReason = "VirtualMachineProvisioned"
	// VMSetCondReasonVirtualMachineCountZero - no virtual machines requested
	VMSetCondReasonVirtualMachineCountZero ospdirectorshared.ConditionReason = "VirtualMachineCountZero"

	// VMSetCondReasonPersitentVolumeClaimNotFound - Persitent Volume Claim Not Found
	VMSetCondReasonPersitentVolumeClaimNotFound ospdirectorshared.ConditionReason = "PersitentVolumeClaimNotFound"
	// VMSetCondReasonPersitentVolumeClaimError - Persitent Volume Claim error
	VMSetCondReasonPersitentVolumeClaimError ospdirectorshared.ConditionReason = "PersitentVolumeClaimError"
	// VMSetCondReasonPersitentVolumeClaimCreating - Persitent Volume Claim create in progress
	VMSetCondReasonPersitentVolumeClaimCreating ospdirectorshared.ConditionReason = "PersitentVolumeClaimCreating"
	// VMSetCondReasonBaseImageNotReady - VM base image not ready
	VMSetCondReasonBaseImageNotReady ospdirectorshared.ConditionReason = "BaseImageNotReady"
)

// Host -
type Host struct {
	Hostname          string                                           `json:"hostname"`
	HostRef           string                                           `json:"hostRef"`
	DomainName        string                                           `json:"domainName"`
	DomainNameUniq    string                                           `json:"domainNameUniq"`
	IPAddress         string                                           `json:"ipAddress"`
	NetworkDataSecret string                                           `json:"networkDataSecret"`
	BaseImageName     string                                           `json:"baseImageName"`
	Labels            map[string]string                                `json:"labels"`
	NAD               map[string]networkv1.NetworkAttachmentDefinition `json:"nad"`
}

// IsReady - Is this resource in its fully-configured (quiesced) state?
func (instance *OpenStackVMSet) IsReady() bool {
	cond := instance.Status.Conditions.InitCondition()

	return cond.Reason == VMSetCondReasonProvisioned || cond.Reason == VMSetCondReasonVirtualMachineCountZero
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=osvmset;osvmsets;osvms
// +operator-sdk:csv:customresourcedefinitions:displayName="OpenStack VMSet"
// +kubebuilder:printcolumn:name="Cores",type="integer",JSONPath=".spec.cores",description="Cores"
// +kubebuilder:printcolumn:name="RAM",type="integer",JSONPath=".spec.memory",description="RAM in GB"
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".spec.vmCount",description="Desired"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.provisioningStatus.readyCount",description="Ready"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.provisioningStatus.state",description="Status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.provisioningStatus.reason",description="Reason"

// OpenStackVMSet represents a set of virtual machines hosts for a specific role within the Overcloud deployment
type OpenStackVMSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenStackVMSetSpec   `json:"spec,omitempty"`
	Status OpenStackVMSetStatus `json:"status,omitempty"`
}

// GetHostnames -
func (instance OpenStackVMSet) GetHostnames() map[string]string {
	ret := make(map[string]string)
	for _, val := range instance.Status.VMHosts {
		ret[val.Hostname] = val.HostRef
	}
	return ret
}

// +kubebuilder:object:root=true

// OpenStackVMSetList contains a list of OpenStackVMSet
type OpenStackVMSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenStackVMSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenStackVMSet{}, &OpenStackVMSetList{})
}
