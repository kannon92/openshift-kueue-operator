/*
Copyright 2024.

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
	configapi "sigs.k8s.io/kueue/apis/config/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KueueSpec defines the desired state of Kueue
type KueueSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Kueue *KueueOperand `json:"kueue,omitempty"`
}

type KueueOperand struct {
	// The config that is persisted to a config map
	Config KueueConfiguration `json:"config"`
	// Image
	Image string `json:"image"`
}

type KueueConfiguration struct {
	// WaitForPodsReady configures gang admission
	// optional
	WaitForPodsReady *configapi.WaitForPodsReady `json:"waitForPodsReady,omitempty"`
	// Integrations are the types of integrations Kueue will manager
	// Required
	Integrations configapi.Integrations `json:"integrations"`
	// Feature gates are advanced features for Kueue
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
}

// KueueStatus defines the observed state of Kueue
type KueueStatus struct {
	KueueReady bool `json:"kueueReady"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Kueue is the Schema for the kueue API
type Kueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KueueSpec   `json:"spec,omitempty"`
	Status KueueStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KueueList contains a list of Kueue
type KueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kueue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kueue{}, &KueueList{})
}
