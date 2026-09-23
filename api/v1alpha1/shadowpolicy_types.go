/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ShadowPolicySpec defines the desired state of ShadowPolicy
type ShadowPolicySpec struct {
	SourceService    string   `json:"sourceService"`
	TargetService    string   `json:"targetService"`
	MirrorPercentage int32    `json:"mirrorPercentage"`
	IgnoredFields    []string `json:"ignoredFields,omitempty"`
}

// ShadowPolicyStatus defines the observed state of ShadowPolicy.
type ShadowPolicyStatus struct {
	Active                bool               `json:"active"`
	TotalRequestsMirrored int64              `json:"totalRequestsMirrored,omitempty"`
	Conditions            []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ShadowPolicy is the Schema for the shadowpolicies API
type ShadowPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShadowPolicySpec   `json:"spec,omitempty"`
	Status ShadowPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ShadowPolicyList contains a list of ShadowPolicy
type ShadowPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShadowPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &ShadowPolicy{}, &ShadowPolicyList{})
		return nil
	})
}
