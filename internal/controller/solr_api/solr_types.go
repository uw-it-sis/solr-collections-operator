package solr_api

// ClusterStatus is a data structure for holding the status of a Solr cluster
type ClusterStatus struct {
	Collections map[string]Collection
	Aliases     map[string]string
}

// Collection is a data structure for holding the status of a particular collection.
type Collection struct {
	// The name of the collection (with the blue/green suffix if appropriate)
	Name string
	// The current replication factor of the collection.
	ReplicationFactor int32
	// The number of replicas currently instantiated
	ReplicaCount int32
	// The name of the configuration used to create the collection
	ConfigName string
}
