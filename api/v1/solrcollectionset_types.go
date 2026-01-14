/*
Copyright 2025.

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

package v1

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultSolrCollectionSetActive           = true
	DefaultSolrCollectionSetCleanupEnabled   = false
	DefaultSolrCollectionSetBlueGreenEnabled = true
	DefaultSolrCollectionReplicationFactor   = int32(1)
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SolrCollectionSetSpec defines the desired state of SolrCollectionSet
type SolrCollectionSetSpec struct {
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// SolrClusterName The name of Solr Cluster to which this cluster set belongs. This value is really just informational.
	SolrClusterName string `json:"clusterName"`

	// SolrClusterUrl The URL to use to interact with the Solr cluster. If omitted defaults to `http://<name>-solrcloud:8389/solr/admin
	// +optional
	SolrClusterUrl string `json:"clusterUrl"`

	// SecretRef The name of the Kubernetes Secret that stores the basic auth secret used to call the Solr API.
	// This secret must be in the same namespace as the collections operator.
	// It should be hashed in the format that Solr expects.
	SecretRef string `json:"secretName"`

	// Active Determines if the CollectionSet is being actively managed or management has been paused
	// +optional
	// +default:true
	Active *bool `json:"active"`

	// ReplicationFactor The replication factor of the collections in the set
	// +optional
	// +default:1
	ReplicationFactor *int32 `json:"replicationFactor"`

	// BlueGreenEnabled Determines if the _blue/_green strategy for managing collections is used.
	// +optional
	// +default:true
	BlueGreenEnabled *bool `json:"blueGreenEnabled"`

	// CleanupEnabled Determines if collections which aren't in the spec are deleted. If this is false you could deploy
	// multiple collection sets on the same Solr cluster. Otherwise, during the reconcile process collections that
	// aren't in the spec would be removed.
	// +optional
	// +default:false
	CleanupEnabled *bool `json:"cleanupEnabled"`

	// Collections The collections that will be managed.
	//+listType:=map
	//+listMapKey:=name
	Collections []SolrCollection `json:"collections"`
}

// +kubebuilder:validation:MinProperties:=0
// +kubebuilder:validation:MaxProperties:=100
type SolrCollection struct {
	// The full name of the managed collection.
	//
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=100
	Name string `json:"name"`

	// The name of alias that will be created for this collection. If blue/green isn't enabled this will be the same as
	// name and no alias will actually be created (as it isn't necessary).
	//
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=100
	// +optional
	Alias string `json:"alias"`

	// configsetName The name of the Kubernetes configmap that contains the schema for this collection. If not provided
	// this will be the same as "alias".
	//
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=100
	// +optional
	ConfigsetName string `json:"configsetName,omitempty"`
}

// SolrCollectionSetStatus defines the observed state of SolrCollectionSet.
type SolrCollectionSetStatus struct {
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	// conditions represent the current state of the SolrCollectionSet resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// This collection should be treated as a map with a key of 'type'
	//
	// This operator has only one condition: Stable (ie is the collection set stable?)
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ReplicationFactor is the replication factor of the collection set. (Currently it's assumed that all collections
	// in a set have the same replication factor)
	ReplicationFactor int32 `json:"replicationFactor"`

	// ReadyRatio is the ratio of specified collections to collections provisioned
	ReadyRatio string `json:"readyRatio"`

	// ScaleStatus is the overall scaling status of the collection set. V
	ScaleStatus string `json:"scaleStatus"`

	// SolrNodes contain the statuses of each solr node running in this solr cloud.
	// +optional
	//+listType:=map
	//+listMapKey:=instanceName
	SolrCollections []SolrCollectionStatus `json:"collections"`
}

// SolrCollectionStatus defines the observed state of a SolrCollection.
type SolrCollectionStatus struct {
	// Name is the specified name of the collection. This omits the blue/green suffix if blue/green is enabled
	Name string `json:"name"`
	// InstanceName is the name of this instance of the collection if blue/green is active or the same as Name if not.
	InstanceName string `json:"instanceName"`
	// ConfigSet is the name of the config set the collection is configured with
	ConfigSet string `json:"configset"`
	// Exists indicates whether the collection has been created in the Solr cluster
	Exists bool `json:"exists"`
	// Active indicates the collection is active in the sense that it's actively being used because an alias is pointing
	// to it or blue/green not enabled
	Active bool `json:"active"`
	// BlueGreen indicates whether this is a blue/green collection or not
	BlueGreen bool `json:"blueGreen"`
	// ReplicationFactor is the actual replication factor of the collection (vs the specified replication factor on the set)
	ReplicationFactor int32 `json:"replicationFactor"`
	// ReplicaCount is the number of replicas of the collection
	ReplicaCount int32 `json:"replicas"`
	// ReplicationStatus is a string representing the desired number of replicas vs the actual number ...
	ReplicationStatus string `json:"replicationStatus"`
}

// WithDefaults set default values when not defined in the spec.
func (sc *SolrCollectionSet) WithDefaults(logger logr.Logger) bool {
	var changedDefaults = sc.Spec.withDefaults(logger)
	var changedCollections = sc.SetCollectionDefaults(logger)
	return changedDefaults || changedCollections
}

func (spec *SolrCollectionSetSpec) withDefaults(logger logr.Logger) (changed bool) {
	if spec.Active == nil {
		changed = true
		r := DefaultSolrCollectionSetActive
		spec.Active = &r
	}

	if spec.BlueGreenEnabled == nil {
		changed = true
		r := DefaultSolrCollectionSetBlueGreenEnabled
		spec.BlueGreenEnabled = &r
	}

	if spec.CleanupEnabled == nil {
		changed = true
		r := DefaultSolrCollectionSetCleanupEnabled
		spec.CleanupEnabled = &r
	}

	if spec.ReplicationFactor == nil {
		changed = true
		r := DefaultSolrCollectionReplicationFactor
		spec.ReplicationFactor = &r
	}

	return changed
}

// SetCollectionDefaults sets collection defaults
func (sc SolrCollectionSet) SetCollectionDefaults(logger logr.Logger) (changed bool) {
	for i := range sc.Spec.Collections {
		// range copies the collection so use the index instead ....
		if sc.Spec.Collections[i].ConfigsetName == "" {
			sc.Spec.Collections[i].ConfigsetName = sc.Spec.Collections[i].Name
			changed = true
		}
		if sc.Spec.Collections[i].Alias == "" {
			sc.Spec.Collections[i].Alias = sc.Spec.Collections[i].Name
			changed = true
		}
	}
	return changed
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=collections
// +kubebuilder:categories=all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CLUSTER",type="string",JSONPath=".spec.clusterName",description="The name of the Solr cluster"
// +kubebuilder:printcolumn:name="ACTIVE",type="boolean",JSONPath=".spec.active",description="Is the cluster being actively managed"
// +kubebuilder:printcolumn:name="SCALEING",type="string",JSONPath=".status.scaleStatus",description="The overall scaling status of the collection set."
// +kubebuilder:printcolumn:name="COLS",type="string",JSONPath=".status.readyRatio",description="The ratio of defined vs provisioned collections in the set"
// +kubebuilder:printcolumn:name="R-FAC",type="integer",JSONPath=".spec.replicationFactor",description="The replication factor of the collection set"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
//
// SolrCollectionSet is the Schema for the solrcollectionsets API
type SolrCollectionSet struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SolrCollectionSet
	// +required
	Spec SolrCollectionSetSpec `json:"spec"`

	// status defines the observed state of SolrCollectionSet
	// +optional
	Status SolrCollectionSetStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
// SolrCollectionSetList contains a list of SolrCollectionSet
type SolrCollectionSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SolrCollectionSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolrCollectionSet{}, &SolrCollectionSetList{})
}
