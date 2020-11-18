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

// ProvisionServerSpec defines the desired state of ProvisionServer
type ProvisionServerSpec struct {
	// The port on which the Apache server should listen
	Port int `json:"port"`
	// URL to *gzipped* RHEL qcow2 image (TODO: support uncompressed -- current implementation is Metal3 pattern)
	RhelImageURL string `json:"rhelImageUrl"`
}

// ProvisionServerStatus defines the observed state of ProvisionServer
type ProvisionServerStatus struct {
	// URL of provisioning image on underlying Apache web server
	LocalImageURL string `json:"localImageUrl,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ProvisionServer is the Schema for the provisionservers API
type ProvisionServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProvisionServerSpec   `json:"spec,omitempty"`
	Status ProvisionServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProvisionServerList contains a list of ProvisionServer
type ProvisionServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProvisionServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProvisionServer{}, &ProvisionServerList{})
}
