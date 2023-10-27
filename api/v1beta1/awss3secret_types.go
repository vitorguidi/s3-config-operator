/*
Copyright 2023.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AwsS3SecretSpec defines the desired state of AwsS3Secret
type AwsS3SecretSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of AwsS3Secret. Edit awss3secret_types.go to remove/update
	S3bucket   string `json:"s3url,omitempty"`
	S3file     string `json:"s3url,omitempty"`
	SecretName string `json:"secretName,omitempty"`
}

// AwsS3SecretStatus defines the observed state of AwsS3Secret
type AwsS3SecretStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AwsS3Secret is the Schema for the awss3secrets API
// +kubebuilder:subresource:status
type AwsS3Secret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec        AwsS3SecretSpec   `json:"spec,omitempty"`
	Status      AwsS3SecretStatus `json:"status,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

//+kubebuilder:object:root=true

// AwsS3SecretList contains a list of AwsS3Secret
type AwsS3SecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AwsS3Secret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AwsS3Secret{}, &AwsS3SecretList{})
}
