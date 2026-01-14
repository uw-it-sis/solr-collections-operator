package solr_api

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"strings"

	"fmt"
	"io"
	"net/http"

	"k8s.io/apimachinery/pkg/util/json"
)

// SolrClient is a basic auth client for the Solr API.
type SolrClient struct {
	Username string
	Password string
	Url      string
}

type ReplicationAdjustment struct {
	CurrentCount int32 // The current number of replicas
	TargetCount  int32 // The desired number of replicas
}

func (r *SolrClient) GetClusterStatus(ctx context.Context) (ClusterStatus, error) {
	//logger := log.FromContext(ctx)

	client := &http.Client{}

	url := fmt.Sprintf("%s/admin/collections?action=CLUSTERSTATUS", r.Url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ClusterStatus{}, err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return ClusterStatus{}, err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return ClusterStatus{}, fmt.Errorf("could not get cluster status [%s] [%s]", resp.Status, msg)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ClusterStatus{}, err
	}

	// Read the response string into a map data structure ....
	var jsonResponse map[string]interface{}
	e := json.Unmarshal(body, &jsonResponse)
	if e != nil {
		return ClusterStatus{}, e
	}

	var jsonCluster = jsonResponse["cluster"]
	var jsonAliases = jsonCluster.(map[string]interface{})["aliases"]
	var jsonCollections = jsonCluster.(map[string]interface{})["collections"]

	aliases := make(map[string]string)
	collections := make(map[string]Collection)

	// Map the aliases ...
	if jsonAliases != nil {
		for key, value := range jsonAliases.(map[string]interface{}) {
			aliases[key] = value.(string)
		}
	}
	// Map the collections ...
	if jsonCollections != nil {
		for collection, value := range jsonCollections.(map[string]interface{}) {

			var rawReplicationFactor = value.(map[string]interface{})["replicationFactor"]
			var replicaCount int32
			var replicationFactor int32

			replicaCount = countReplicas(value)
			replicationFactor = interfaceToInt32(rawReplicationFactor)

			collections[collection] = Collection{
				Name:              collection,
				ConfigName:        value.(map[string]interface{})["configName"].(string),
				ReplicationFactor: replicationFactor,
				ReplicaCount:      replicaCount,
			}
		}
	}

	clusterStatus := ClusterStatus{
		Aliases:     aliases,
		Collections: collections,
	}

	return clusterStatus, nil
}

// Gets the config sets that are present in Solr.
func (r *SolrClient) GetConfigSets() ([]string, error) {
	client := &http.Client{}

	url := fmt.Sprintf("%s/admin/configs?action=LIST&wt=json", r.Url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return nil, fmt.Errorf("could not get configsets [%s] [%s]", resp.Status, msg)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Read the response string into a map data structure ....
	var jsonResponse map[string]interface{}
	err = json.Unmarshal(body, &jsonResponse)
	if err != nil {
		return nil, err
	}

	var configSetsJson = jsonResponse["configSets"]
	// Get the list of existing config sets ...
	var configSets []string
	for _, value := range configSetsJson.([]interface{}) {
		configSets = append(configSets, value.(string))
	}

	return configSets, nil
}

// UploadConfigSet creates a configset
func (r *SolrClient) UploadConfigSet(configSetName string, body []byte) error {
	client := &http.Client{}

	// https://solr.apache.org/guide/solr/latest/configuration-guide/configsets-api.html
	url := fmt.Sprintf("%s/admin/configs?action=UPLOAD&name=%s&overwrite=true&cleanup=true&wt=json", r.Url, configSetName)

	bodyReader := bytes.NewBuffer(body)
	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("create config set %s failed with [%s] [%s]", configSetName, resp.Status, msg)
	}

	return nil
}

// DeleteConfigSet deletes the given config set from Solr ...
func (r *SolrClient) DeleteConfigSet(configSetName string) error {
	client := &http.Client{}

	url := fmt.Sprintf("%s/admin/configs?action=DELETE&name=%s&wt=json", r.Url, configSetName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("delete config set %s failed with [%s] [%s]", configSetName, resp.Status, msg)
	}

	return nil
}

// SetReplicationFactor adjusts the replication factor of a collection to the given value ...
func (r *SolrClient) SetReplicationFactor(collectionName string, replicationFactor int32) error {
	client := &http.Client{}
	url := fmt.Sprintf("%s/admin/collections?action=MODIFYCOLLECTION&collection=%s&replicationFactor=%d&wt=json",
		r.Url, collectionName, replicationFactor)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("set replication factor failed on collection [%s] with [%s] [%s]",
			collectionName, resp.Status, msg)
	}

	return nil
}

// AddReplicas adds the given number of replicas
func (r *SolrClient) AddReplicas(collectionName string, increaseCount int32) (isScaling bool, error error) {
	client := &http.Client{}

	url := fmt.Sprintf("%s/admin/collections?action=ADDREPLICA&collection=%s&shard=shard1&nrtReplicas=%d&wt=json",
		r.Url, collectionName, increaseCount)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return false, fmt.Errorf("request failed")
	}

	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		// During the scale out process the Kubernetes has to provision new worker nodes (one per replica), then Solr
		// has to put a node on it, then a replica can be added. This takes a while and calls to add replicas will fail
		// while this happens. To accommodate these errors and not try again too aggressively we attempt to identify
		// the error. Brittle, but necessary.

		if strings.Contains(msg, "Not enough eligible nodes") {
			isScaling = true
		}

		if !isScaling {
			return isScaling, fmt.Errorf("add replicas failed for collection [%s] with [%s] [%s]",
				collectionName, resp.Status, msg)
		} else {
			return isScaling, fmt.Errorf("add replicas failed for collection [%s] because there aren't enough nodes",
				collectionName)
		}
	}

	return isScaling, nil
}

// RemoveReplicas removes the given number of replicas
func (r *SolrClient) RemoveReplicas(collectionName string, decreaseCount int32) error {
	client := &http.Client{}
	// Multiple replicas can be deleted from a specific shard if the associated collection and shard names are provided,
	// along with a count of the replicas to delete.
	url := fmt.Sprintf("%s/admin/collections?action=DELETEREPLICA&collection=%s&shard=shard1&count=%d&wt=json",
		r.Url, collectionName, decreaseCount)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("remove replicas failed on collection %s failed with [%s] [%s]", collectionName, resp.Status, msg)
	}

	return nil
}

// CreateCollection creates a collection ...
func (r *SolrClient) CreateCollection(collectionName string, configSetName string, replicationFactor int32) error {
	client := &http.Client{}
	// http://localhost:8983/solr/admin/collections?action=CREATE&name=techproducts_v2&collection.configName=techproducts&numShards=1
	url := fmt.Sprintf("%s/admin/collections?action=CREATE&name=%s&collection.configName=%s&numShards=1&replicationFactor=%d&autoAddReplicas=true&wt=json",
		r.Url, collectionName, configSetName, replicationFactor)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("create collection %s failed with [%s] [%s]", collectionName, resp.Status, msg)
	}

	return nil
}

// AssignAlias creates an alias for the given collection ...
func (r *SolrClient) AssignAlias(alias string, collectionName string) error {
	client := &http.Client{}
	// /admin/collections?action=CREATEALIAS&name=name&collections=collectionlist
	url := fmt.Sprintf("%s/admin/collections?action=CREATEALIAS&name=%s&collections=%s",
		r.Url, alias, collectionName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("create alias [%s] for collection [%s] failed with [%s] [%s]",
			alias, collectionName, resp.Status, msg)
	}

	return nil
}

// DeleteAlias removes the given alias ...
func (r *SolrClient) DeleteAlias(alias string) error {
	client := &http.Client{}
	// http://localhost:8983/solr/admin/collections?action=DELETEALIAS&name=testalias
	url := fmt.Sprintf("%s/admin/collections?action=DELETEALIAS&name=%s", r.Url, alias)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("remove alias [%s] failed [%s] [%s]", alias, resp.Status, msg)
	}

	return nil
}

// ReloadCollection causes a Solr collection to be reloaded
func (r *SolrClient) ReloadCollection(collectionName string) error {
	client := &http.Client{}

	url := fmt.Sprintf("%s/admin/collections?action=RELOAD&name=%s", r.Url, collectionName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("relocal collection %s failed with [%s] [%s]", collectionName, resp.Status, msg)
	}

	return nil
}

// DeleteCollection deletes the given collection from Solr ...
func (r *SolrClient) DeleteCollection(collectionName string) error {
	client := &http.Client{}

	url := fmt.Sprintf("%s/admin/collections?action=DELETE&name=%s", r.Url, collectionName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("delete collection [%s] failed with [%s] [%s]", collectionName, resp.Status, msg)
	}

	return nil
}

// Query performs a query against the given collection and returns the results in a list of map[string]interface{}
func (r *SolrClient) Query(collectionName string, query string) ([]map[string]interface{}, error) {
	client := &http.Client{}

	url := fmt.Sprintf("%s/%s/select?q.op=OR&rows=1000&q=%s", r.Url, collectionName, query)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	r.addBasicAuth(req)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	// If the response wasn't a 200 then fish out the error ...
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return nil, fmt.Errorf("query to collection [%s] failed with [%s] [%s]", collectionName, resp.Status, msg)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Read the response string into a map data structure ....
	var jsonResponse map[string]interface{}
	e := json.Unmarshal(body, &jsonResponse)
	if e != nil {
		return nil, e
	}

	var response = jsonResponse["response"]
	var docs = response.(map[string]interface{})["docs"]

	var docsOut []map[string]interface{}
	for _, doc := range docs.([]interface{}) {
		var rec = make(map[string]interface{})
		for key, value := range doc.(map[string]interface{}) {
			rec[key] = value
		}
		docsOut = append(docsOut, rec)
	}

	return docsOut, nil
}

// WriteRecord writes a single solr record to the given collection ...
func (r *SolrClient) WriteRecord(collectionName string, record string) error {
	client := &http.Client{}

	url := fmt.Sprintf("%s/%s/update?commit=true", r.Url, collectionName)

	bodyReader := bytes.NewBuffer([]byte(fmt.Sprintf("[%s]", record)))
	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return err
	}

	r.addBasicAuth(req)

	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	// If the response isn't 200 then parse the response for the error message ...
	if resp.StatusCode != 200 {
		msg, _ := parseError(resp.Body)
		return fmt.Errorf("write to collection %s failed with [%s] [%s]", collectionName, resp.Status, msg)
	}

	return nil
}

// countReplicas counts replicas in a collection json object ...
func countReplicas(collection interface{}) (count int32) {
	var shards = collection.(map[string]interface{})["shards"]
	var shard1 = shards.(map[string]interface{})["shard1"]
	var replicas = shard1.(map[string]interface{})["replicas"]
	for range replicas.(map[string]interface{}) {
		count++
	}
	return count
}

// parseError fishes the error message out of an error response ...
func parseError(reader io.Reader) (string, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return "failed to read", err
	}
	var jsonResponse map[string]interface{}
	err = json.Unmarshal(body, &jsonResponse)
	if err != nil {
		return "couldn't unmarshall the response body", err
	}

	e := jsonResponse["error"]
	msg := e.(map[string]interface{})["msg"]
	return msg.(string), nil
}

// addBasicAuth Add basic auth to the given request ...
func (r *SolrClient) addBasicAuth(req *http.Request) {
	username := r.Username
	password := r.Password
	req.SetBasicAuth(username, password)
}

// interfaceToInt32 Deals with turning JSON numbers into int32s ...
func interfaceToInt32(i interface{}) int32 {
	var result int32 = 0
	switch reflect.TypeOf(i).Kind().String() {
	case "int32":
		result = i.(int32)
	case "int64":
		result = int32(i.(int64))
	case "string":
		// Ignoring the error here. Probably unwise.
		some, _ := strconv.Atoi(i.(string))
		result = int32(some)
	}
	return result
}
