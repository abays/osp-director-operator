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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenStackProvisionServerSpec defines the desired state of OpenStackProvisionServer
type OpenStackProvisionServerSpec struct {
	// The port on which the Apache server should listen
	Port int `json:"port"`
	// An optional interface to use instead of the cluster's default provisioning interface (if any)
	Interface string `json:"interface,omitempty"`
	// URL to *gzipped* RHEL qcow2 image (TODO: support uncompressed -- current implementation is Metal3 pattern)
	BaseImageURL string `json:"baseImageUrl"`
}

// OpenStackProvisionServerStatus defines the observed state of OpenStackProvisionServer
type OpenStackProvisionServerStatus struct {
	// IP of the provisioning interface on the node running the ProvisionServer pod
	ProvisionIP string `json:"provisionIp,omitempty"`
	// URL of provisioning image on underlying Apache web server
	LocalImageURL string `json:"localImageUrl,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=osprovserver;osprovservers
// +operator-sdk:csv:customresourcedefinitions:displayName="OpenStack ProvisionServer"

// OpenStackProvisionServer is the Schema for the openstackprovisionservers API
type OpenStackProvisionServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenStackProvisionServerSpec   `json:"spec,omitempty"`
	Status OpenStackProvisionServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenStackProvisionServerList contains a list of OpenStackProvisionServer
type OpenStackProvisionServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenStackProvisionServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenStackProvisionServer{}, &OpenStackProvisionServerList{})
}
