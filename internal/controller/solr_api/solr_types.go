package solr_api

// ClusterStatus is a data structure for holding the status of a Solr cluster
type ClusterStatus struct {
	Collections map[string]Collection
	Aliases     map[string]string
}

// Collection is a data structure for holding the status of a particular collection.
type Collection struct {
	Name string
	// The current replication factor of the collection.
	ReplicationFactor int32
	// The number of replicas currently instantiated
	ReplicaCount int32
	// The name of the configuration used to create the collection
	ConfigName string
}

//{
//  "responseHeader":{
//    "status":0,
//    "QTime":1
//  },
//  "cluster":{
//    "live_nodes":["testing-solrcloud-1.testing-solrcloud-headless.default:8983_solr","testing-solrcloud-0.testing-solrcloud-headless.default:8983_solr"],
//    "collections":{
//      "test4":{
//        "pullReplicas":"0",
//        "configName":"_default",
//        "replicationFactor":"2",
//        "router":{
//          "name":"compositeId",
//          "field":null
//        },
//        "nrtReplicas":"2",
//        "tlogReplicas":"0",
//        "shards":{
//          "shard1":{
//            "range":"80000000-7fffffff",
//            "state":"active",
//            "replicas":{
//              "core_node2":{
//                "core":"test4_shard1_replica_n1",
//                "node_name":"testing-solrcloud-1.testing-solrcloud-headless.default:8983_solr",
//                "type":"NRT",
//                "state":"active",
//                "leader":"true",
//                "force_set_state":"false",
//                "base_url":"http://testing-solrcloud-1.testing-solrcloud-headless.default:8983/solr"
//              },
//              "core_node4":{
//                "core":"test4_shard1_replica_n3",
//                "node_name":"testing-solrcloud-0.testing-solrcloud-headless.default:8983_solr",
//                "type":"NRT",
//                "state":"active",
//                "force_set_state":"false",
//                "base_url":"http://testing-solrcloud-0.testing-solrcloud-headless.default:8983/solr"
//              }
//            },
//            "health":"GREEN"
//          }
//        },
//        "health":"GREEN",
//        "znodeVersion":26,
//        "creationTimeMillis":1766083593197
//      },
//      "buildInfo_blue":{
//        "pullReplicas":"0",
//        "configName":"buildInfo",
//        "replicationFactor":1,
//        "router":{
//          "name":"compositeId"
//        },
//        "nrtReplicas":1,
//        "tlogReplicas":"0",
//        "shards":{
//          "shard1":{
//            "range":"80000000-7fffffff",
//            "state":"active",
//            "replicas":{
//              "core_node2":{
//                "core":"buildInfo_blue_shard1_replica_n1",
//                "node_name":"testing-solrcloud-0.testing-solrcloud-headless.default:8983_solr",
//                "type":"NRT",
//                "state":"active",
//                "leader":"true",
//                "force_set_state":"false",
//                "base_url":"http://testing-solrcloud-0.testing-solrcloud-headless.default:8983/solr"
//              }
//            },
//            "health":"GREEN"
//          }
//        },
//        "health":"GREEN",
//        "znodeVersion":8,
//        "creationTimeMillis":1766004824041,
//        "aliases":["buildInfo"]
//      }
//    },
//    "aliases":{
//      "buildInfo":"buildInfo_blue"
//    },
//    "properties":{
//      "plugin":{
//        ".placement-plugin":{
//          "name":".placement-plugin",
//          "class":"org.apache.solr.cluster.placement.plugins.AffinityPlacementFactory",
//          "config":{
//            "minimalFreeDiskGB":0
//          }
//        }
//      }
//    },
//    "roles":{ }
//  }
//}
