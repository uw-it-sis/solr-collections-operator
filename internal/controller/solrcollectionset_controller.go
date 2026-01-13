package controller

import (
	"context"
	"crypto/md5"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	solr "github.com/uw-it-sis/solr-collections-operator/internal/controller/solr_api"

	"github.com/uw-it-sis/solr-collections-operator/internal/controller/utils"

	solrCollectionSet "github.com/uw-it-sis/solr-collections-operator/api/v1"
)

// To embed files into the binary. (In this case the schema files for the config set checksums)
import _ "embed"

// SolrCollectionSet condition and event types/reasons ...
const (
	// Conditions ...

	// typeSolrCollectionSetStable indicates the specified state and the cluster state are aligned and no errors have
	// been encountered during the reconcile
	typeSolrCollectionSetStable = "Stable"

	// Condition reasons ...

	// reasonSolrCollectionSetStable is used when the collection set is stable
	reasonSolrCollectionSetStable = "stable"
	// reasonSolrCollectionSetInitializing means the collection set is being initialized
	reasonSolrCollectionSetInitializing = "initializing"
	// reasonSolrCollectionSetScalingIn means collection replicas are being reduced
	reasonSolrCollectionSetScalingIn = "scalingIn"
	// reasonSolrCollectionSetScalingOut means collection replicas are being increased
	reasonSolrCollectionSetScalingOut = "scalingOut"
	// reasonSolrCollectionAddingCollections means collections are being added
	reasonSolrCollectionAddingCollections = "addingCollections"
	// reasonSolrCollectionRemovingCollections means collection are being removed
	reasonSolrCollectionRemovingCollections = "removingCollections"
	// reasonSolrCollectionReplicationFactorMismatch means the replication factor defined in the spec doesn't match a
	// collection's replication factor
	reasonSolrCollectionReplicationFactorMismatch = "replicationFactorMismatch"

	// reasonSolrCollectionSetReconcileError means an error has been encountered during the reconcile process
	reasonSolrCollectionSetReconcileError = "errorEncountered"

	// Events ...

	// eventSolrCollectionSetInitializing is an event which indicates that the collection set is being newly initialized
	eventSolrCollectionSetInitializing = "Initializing"
	// eventSolrCollectionSetScaleOut  is an event which indicates that a scale out operation has started
	eventSolrCollectionSetScaleOut = "ScaleOut"
	// eventSolrCollectionSetScaleIn  is an event which indicates that a scale in operation has started
	eventSolrCollectionSetScaleIn = "ScaleIn"
	// eventSolrCollectionSetAddingCollection  is an event which indicates collections are being added
	eventSolrCollectionSetAddingCollection = "AddingCollection"
	// eventSolrCollectionSetRemovingCollection is an event which indicates collections are being removed
	eventSolrCollectionSetRemovingCollection = "RemovingCollection"
)

const (
	// this has a placeholder for the collection set name ...
	configChecksumsCollectionNameTemplate = "_%sChecksums"
	configChecksumsConfigSetName          = "_checksums"
)

const (
	errorRequeueSeconds   = 60
	backoffRequeueSeconds = 20
)

// This annotation is what causes the files to become embedded ...
// vvvvvvv
//
//go:embed checksum_collection_configset
var checksumCollectionSchema embed.FS

var solrClient solr.SolrClient

// SolrCollectionSetReconciler reconciles a SolrCollectionSet object
type SolrCollectionSetReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Access controls for the resources ...
// +kubebuilder:rbac:groups=solrcollections.solr.sis.uw.edu,resources=solrcollectionsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=solrcollections.solr.sis.uw.edu,resources=solrcollectionsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=solrcollections.solr.sis.uw.edu,resources=solrcollectionsets/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//
// Reconcile is part of the main kubernetes reconciliation loop which aims to move the current state of the cluster
// closer to the desired state. To do that it compares the state specified by the SolrCollectionSet object against the
// actual cluster state, and then performs operations to make the cluster state reflect the state specified by
// the user.
//
// It's important to understand that the Reconcile method gets called when a change is made on a (SolrCollectionSet) CRD
// in Kubernetes or when the Status of the CRD is updated within the Reconcile loop, so it's very possible to create
// an infinite loop by, for instance, updating the Status on each loop. With that in mind, the state of the Reconciling
// Condition works like this. It starts out as False, and remains so until a diff between the spec and and the cluster
// is found. Once the change has been applied and the difference is resolved the state is set back to False, and remains
// that way until a diff is found.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *SolrCollectionSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the collection set (aka the collection set spec) via the Kubernetes API ...
	collectionSetSpec := &solrCollectionSet.SolrCollectionSet{}
	err := r.Get(ctx, req.NamespacedName, collectionSetSpec)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			// TODO: If the CRD just disappears Solr kinda needs to be cleaned up, but how do you know what to clean
			//       up if the spec isn't available?
			logger.Info("SolrCollectionSet resource not found. Ignoring since object must be deleted")
			return requeue()
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get SolrCollectionSet ")
		// RequeueOnError() isn't possible here ...
		return requeue()
	}

	// Initialize status Conditions if not yet present ...
	if len(collectionSetSpec.Status.Conditions) == 0 {
		meta.SetStatusCondition(&collectionSetSpec.Status.Conditions, metav1.Condition{
			Type:    typeSolrCollectionSetStable,
			Status:  metav1.ConditionUnknown,
			Reason:  reasonSolrCollectionSetInitializing,
			Message: "Bootstrapping the operator",
		})

		// Commit the status update of the collection set in Kubernetes ...
		if err := r.Status().Update(ctx, collectionSetSpec); err != nil {
			logger.Error(err, "failed to update SolrCollectionSet status")
			return requeue()
		}

		// Re-fetch the SolrCollectionSet after updating the status
		if err := r.Get(ctx, req.NamespacedName, collectionSetSpec); err != nil {
			logger.Error(err, "failed to re-fetch SolrCollectionSet")
			return r.RequeueOnError(ctx, req, collectionSetSpec, err)
		}
	}

	// Apply/fill-in defaults for the spec ...
	changed := collectionSetSpec.WithDefaults(logger)
	if changed {
		logger.Info("applying default settings for SolrCollectionSet")
		if err = r.Update(ctx, collectionSetSpec); err != nil {
			logger.Error(err, "failed to update collection set")
			return r.RequeueOnError(ctx, req, collectionSetSpec, err)
		}
		// Immediately requeue so that new values are read ...
		return requeueImmediately()
	}

	// Short-circuit if this collection set isn't active (i.e. being actively managed) ...
	if !*collectionSetSpec.Spec.Active {
		return requeue()
	}

	//
	// Initialize Solr cluster. This method returns a solr.ClusterStatus object representing the current state of the
	// Solr cluster.
	var checksumsCollectionName = fmt.Sprintf(configChecksumsCollectionNameTemplate, collectionSetSpec.Name)
	clusterStatus, isIntializing, err := r.InitializeSolrCluster(ctx, *collectionSetSpec, checksumsCollectionName)
	if err != nil {
		logger.Error(err, "failed to initialize the Solr cluster")
		return r.RequeueOnError(ctx, req, collectionSetSpec, err)
	}
	// Emit the intializing event if Solr is initializing ...
	if isIntializing {
		r.Recorder.Eventf(collectionSetSpec, corev1.EventTypeNormal, eventSolrCollectionSetInitializing,
			"SolrCollectionSpec [%s] is being initialized in namespace [%s]",
			collectionSetSpec.Name, collectionSetSpec.Namespace)
	}

	//
	// Compare the cluster status with the spec and persist the outcome into Kubernetes ...
	//
	err = r.UpdateStatus(ctx, req, collectionSetSpec, clusterStatus)
	if err != nil {
		logger.Error(err, "update status failed")
		return r.RequeueOnError(ctx, req, collectionSetSpec, err)
	}

	//
	// Reconcile config sets ...
	//   (Note: This doesn't update the collection set spec so passing the collection set value vs the pointer)
	//
	err = r.ManageConfigSets(ctx, *collectionSetSpec, checksumsCollectionName)
	if err != nil {
		logger.Error(err, "failed to manage config set")
		return r.RequeueOnError(ctx, req, collectionSetSpec, err)
	}

	//
	// Reconcile collections ...
	//   (Note: This doesn't update the  collection set spec so passing the collection set value vs the pointer)
	changed = r.ManageCollections(ctx, *collectionSetSpec, clusterStatus.Collections, clusterStatus.Aliases)
	if changed {
		// Requeue (i.e. run the reconcile again) to make sure Solr is in a stable state before proceeding.
		return requeueImmediately()
	}

	//
	// Perform scale-out/in ...
	// The number of replicas and the number of worker nodes in the Kubernetes cluster is usually the same. However,
	// during scale out it takes a while for the autoscaler to create nodes on which to schedule additional replicas.
	// That means that AdjustReplicas() will sometime get errors because there aren't Solr nodes available to create
	// replias on (because worker nodes are being created). In that case isScaling will return true.
	//
	isScaling, err := r.AdjustReplicas(ctx, *collectionSetSpec, clusterStatus.Collections)
	if err != nil {
		logger.Error(err, "adjust replicas failed")
		return r.RequeueOnError(ctx, req, collectionSetSpec, err)
	}
	if isScaling {
		return reconcile.Result{RequeueAfter: time.Second * backoffRequeueSeconds}, nil
	}

	return requeue()
}

// InitializeSolrCluster gets the Solr ready to interact with and returns the current state. It's okay to call this
// method repeatedly.
func (r *SolrCollectionSetReconciler) InitializeSolrCluster(ctx context.Context,
	collectionSet solrCollectionSet.SolrCollectionSet,
	checksumsCollectionName string) (clusterStatus solr.ClusterStatus, isInitializing bool, err error) {

	logger := log.FromContext(ctx)

	// If no Solr client has been instantiated then do it ...
	if solrClient == (solr.SolrClient{}) {
		logger.Info("instantiating a solr client")
		secretRef := collectionSet.Spec.SecretRef
		clusterUrl := collectionSet.Spec.SolrClusterUrl
		sc, err := r.makeSolrClient(ctx, secretRef, clusterUrl)
		solrClient = sc
		if err != nil {
			return solr.ClusterStatus{}, false, err
		}
	}

	// Fetch the Solr cluster status from the Solr API ...
	clusterStatus, err = solrClient.GetClusterStatus(ctx)
	if err != nil {
		return solr.ClusterStatus{}, false, err
	}

	// See if the checksums collection exists. If it doesn't, create it ...
	_, exists := clusterStatus.Collections[checksumsCollectionName]
	if !exists {
		// If the checksum collection doesn't exist then the cluster is initializing. There are a couple more things
		// that could be checked as well, but I think this is a pretty good indicator and I don't believe it would be
		// helpful to throw multiples of this event ...
		isInitializing = true
		logger.Info(fmt.Sprintf("Creating collection [%s] for checksums", configChecksumsCollectionNameTemplate))
		err := createChecksumCollection(checksumsCollectionName, *collectionSet.Spec.ReplicationFactor)
		if err != nil {
			logger.Error(err, "failed create checksum collection")
			return solr.ClusterStatus{}, isInitializing, err
		}

		// Re-fetch the Solr cluster status just to provide an update to date status since a collection was added. I
		// suppose it would be more efficient to manually add the collection the response, but it's a pretty low cost
		// operator as far as I can tell ...
		clusterStatus, err = solrClient.GetClusterStatus(ctx)
		if err != nil {
			return solr.ClusterStatus{}, false, err
		}
	}
	return clusterStatus, isInitializing, nil
}

// UpdateStatus applies the given cluster status to the given collection set ...
func (r *SolrCollectionSetReconciler) UpdateStatus(
	ctx context.Context, req ctrl.Request, collectionSet *solrCollectionSet.SolrCollectionSet, clusterStatus solr.ClusterStatus) error {

	logger := log.FromContext(ctx)

	// Create storage for the new/empty status for the collection set  ...
	newStatusObject := solrCollectionSet.SolrCollectionSetStatus{}
	events, err := populateCollectionSetStatus(&newStatusObject, collectionSet, clusterStatus, logger)
	if err != nil {
		return err
	}
	// Emit events if there are any ...
	if len(events) != 0 {
		for eventType, reason := range events {
			r.Recorder.Eventf(collectionSet, corev1.EventTypeNormal, eventType, reason)
		}
	}

	// If the new status object and the old status object differ, then apply the changes. Note that patching the
	// collection set will cause the
	if !reflect.DeepEqual(collectionSet.Status, newStatusObject) {
		oldInstance := collectionSet.DeepCopy()
		collectionSet.Status = newStatusObject
		err = r.Status().Patch(ctx, collectionSet, client.MergeFrom(oldInstance))
		if err != nil {
			logger.Error(err, fmt.Sprintf("failed to save collection set status [%s]", collectionSet.Name))
			return err
		}
	}

	// Re-fetch the SolrCollectionSet after updating the status
	if err := r.Get(ctx, req.NamespacedName, collectionSet); err != nil {
		logger.Error(err, fmt.Sprintf("failed to re-fetch SolrCollectionSet [%s]", collectionSet.Name))
		return err
	}

	return nil
}

// populateCollectionSetStatus populates a collection set status object ...
func populateCollectionSetStatus(
	newStatus *solrCollectionSet.SolrCollectionSetStatus,
	collectionSet *solrCollectionSet.SolrCollectionSet,
	clusterStatus solr.ClusterStatus,
	logger logr.Logger) (events map[string]string, err error) {

	// Storage for events to be returned ...
	events = make(map[string]string)

	// isStable controls the condition and will be true if no adjustments are outstanding and no errors are encountered
	isStable := true

	// Why isn't the collectionSpec set stable ...
	unstableReason := ""

	// Set replication factor in the new spec status ...
	var collectionSetReplicationFactor = *collectionSet.Spec.ReplicationFactor
	newStatus.ReplicationFactor = collectionSetReplicationFactor

	// Look at the overall status of the collections ...
	specifiedCollectionCount := countSpecifiedCollections(collectionSet.Spec.Collections, *collectionSet.Spec.BlueGreenEnabled)
	solrCollectionsCount := countSolrCollections(clusterStatus.Collections)

	if specifiedCollectionCount != solrCollectionsCount {
		isStable = false
		number := abs(int32(specifiedCollectionCount - solrCollectionsCount))
		// If blue/green is enabled then only count each blue/green as 1 collection ...
		if *collectionSet.Spec.BlueGreenEnabled {
			number = number / 2
		}
		if specifiedCollectionCount < solrCollectionsCount {
			unstableReason = reasonSolrCollectionRemovingCollections
			events[eventSolrCollectionSetRemovingCollection] =
				fmt.Sprintf("SolrCollectionSpec [%s] is in namespace [%s] is removing [%d] collections",
					collectionSet.Name, collectionSet.Namespace, number)
		}
		if specifiedCollectionCount > solrCollectionsCount {
			unstableReason = reasonSolrCollectionAddingCollections
			events[eventSolrCollectionSetAddingCollection] =
				fmt.Sprintf("SolrCollectionSpec [%s] is in namespace [%s] is adding [%d] collections",
					collectionSet.Name, collectionSet.Namespace, number)
		}
	}

	// Set the "ready" status. ReadyRatio is the number of collections created / number of collections specified ...
	newStatus.ReadyRatio = fmt.Sprintf("%d/%d", solrCollectionsCount, specifiedCollectionCount)

	//
	// Look at the status of the individual collections ...
	//
	// Reverse map the aliases map (collectionSpec->alias) ...
	var collectionsToAliasesMap = make(map[string]string)
	for alias, collection := range clusterStatus.Aliases {
		collectionsToAliasesMap[collection] = alias
	}

	// Create a SolrSectionStatus object for each specified collectionSpec which only has data from the spec  ...
	var collectionStatusMap = make(map[string]*solrCollectionSet.SolrCollectionStatus)
	for _, collectionSpec := range collectionSet.Spec.Collections {
		collectionName := collectionSpec.Name
		if *collectionSet.Spec.BlueGreenEnabled {
			for _, suffix := range []string{"_blue", "_green"} {
				instanceName := collectionName + suffix
				newItem := newSolrSectionStatus(collectionSpec, instanceName)
				collectionStatusMap[instanceName] = &newItem
			}
		} else {
			// No blue/green here ...
			newItem := newSolrSectionStatus(collectionSpec, "")
			collectionStatusMap[collectionName] = &newItem
		}
	}

	// Iterate through the solr collections from the cluster and update the collection status objects ...
	for name, collection := range clusterStatus.Collections {
		// Only count specified collections (collections that the operator itself uses begin with '_') ...
		if strings.HasPrefix(collection.Name, "_") {
			continue
		}

		isActive := true
		if *collectionSet.Spec.BlueGreenEnabled {
			// Strip the suffix off to learn the collectionSpec name (i.e. the name specified in the spec) ...
			var collectionName = name
			collectionName = strings.TrimSuffix(name, "_blue")
			collectionName = strings.TrimSuffix(name, "_green")
			// See if there's an alias pointing to the collectionSpec ...
			_, exists := collectionsToAliasesMap[collectionName]
			if !exists {
				isActive = false
			}
		}

		// If the replication factor of the collectionSpec doesn't match the replication factor specified in the set then
		// that means the collectionSpec set is unstable ....
		if collectionSetReplicationFactor != collection.ReplicationFactor {
			isStable = false
			unstableReason = reasonSolrCollectionReplicationFactorMismatch
		}

		// replicationStatus is the number of replicas called for by the collectionSpec's replication status vs the number
		// of replicas that are in the cluster ...
		var replicaCount = collection.ReplicaCount
		replicationStatus := fmt.Sprintf("%d/%d", replicaCount, collection.ReplicationFactor)

		if collection.ReplicaCount != collection.ReplicationFactor {
			isStable = false
			if collection.ReplicaCount < collection.ReplicationFactor {
				unstableReason = reasonSolrCollectionSetScalingOut
				events[eventSolrCollectionSetScaleOut] =
					fmt.Sprintf("SolrCollectionSpec [%s] is in namespace [%s] is scaling out from [%d] replicas to [%d]",
						collectionSet.Name, collectionSet.Namespace, collection.ReplicaCount, collection.ReplicationFactor)
			} else if collection.ReplicaCount > collection.ReplicationFactor {
				unstableReason = reasonSolrCollectionSetScalingIn
				events[eventSolrCollectionSetScaleIn] =
					fmt.Sprintf("SolrCollectionSpec [%s] is in namespace [%s] is scaling in from [%d] replicas to [%d]",
						collectionSet.Name, collectionSet.Namespace, collection.ReplicaCount, collection.ReplicationFactor)
			}
		}

		solrCollectionStatus, exists := collectionStatusMap[name]
		if !exists {
			continue
		}

		solrCollectionStatus.ReplicationFactor = collection.ReplicationFactor
		solrCollectionStatus.ReplicaCount = collection.ReplicaCount
		solrCollectionStatus.ReplicationStatus = replicationStatus
		solrCollectionStatus.Active = isActive
		solrCollectionStatus.Exists = true
	}

	// Write the collection status object into the status object ...
	newStatus.SolrCollections = []solrCollectionSet.SolrCollectionStatus{}
	for _, collectionStatus := range collectionStatusMap {
		newStatus.SolrCollections = append(newStatus.SolrCollections, *collectionStatus)
	}

	// Examine conditions ...

	// Map the existing conditions by type for comparison with new conditions ...
	existingConditions := make(map[string]metav1.Condition)
	for _, condition := range collectionSet.Status.Conditions {
		existingConditions[condition.Type] = condition
	}

	// Fix status ...
	var stableStatus = metav1.ConditionTrue
	var stableMessage string
	if isStable {
		// It's a stable reason here, but unstable make more sense about everywhere else ...
		unstableReason = reasonSolrCollectionSetStable
	} else {
		stableStatus = metav1.ConditionFalse
		stableMessage = "Spec and cluster status are not aligned"
	}

	// Make a map of new conditions based on the logic above ...
	newConditions := make(map[string]metav1.Condition)

	newConditions[typeSolrCollectionSetStable] = metav1.Condition{
		Type:    typeSolrCollectionSetStable,
		Status:  stableStatus,
		Reason:  unstableReason,
		Message: stableMessage,
	}

	// Carrying forward conditions which do not exist in the new conditions map. At this point I believe this is mainly
	// just a precaution.
	for t, _ := range existingConditions {
		_, exists := newConditions[t]
		if !exists {
			meta.SetStatusCondition(&newStatus.Conditions, existingConditions[t])
		}
	}

	// Iterate though the condition that were just formulated and apply the to the status ...
	for t, condition := range newConditions {
		// Look for the condition in the existing conditions map ...
		existingCondition, exists := existingConditions[t]
		if exists {
			// Compare them ...
			if conditionsEqual(condition, existingCondition) {
				// If they are identical then carry forward the existing condition as-is ...
				meta.SetStatusCondition(&newStatus.Conditions, existingCondition)
			} else {
				// Otherwise, use the new condition ...
				changed := meta.SetStatusCondition(&newStatus.Conditions, newConditions[t])
				if changed {
					logger.Info(fmt.Sprintf("updated condition [%s] with status [%s]", newConditions[t].Type, newConditions[t].Status))
				}
			}
		} else {
			// If the condition doesn't exist then use the new condition
			changed := meta.SetStatusCondition(&newStatus.Conditions, newConditions[t])
			if changed {
				logger.Info(fmt.Sprintf("added condition [%s] with status [%s]", newConditions[t].Type, newConditions[t].Status))
			}
		}
	}

	return events, nil
}

// newSolrSectionStatus creates and instance of SolrCollectionStatus only data from the spec ...
func newSolrSectionStatus(collectionSpec solrCollectionSet.SolrCollection, instanceName string) solrCollectionSet.SolrCollectionStatus {
	isBlueGreen := false
	// If no instance name is given then assume blue/green
	if instanceName != "" {
		isBlueGreen = true
	}
	return solrCollectionSet.SolrCollectionStatus{
		Name:              collectionSpec.Name,
		InstanceName:      instanceName,
		ConfigSet:         collectionSpec.ConfigsetName,
		ReplicationFactor: 0,
		Exists:            false,
		Active:            false,
		ReplicaCount:      0,
		BlueGreen:         isBlueGreen,
		ReplicationStatus: "--",
	}
}

// conditionsEqual tests if the two given conditions are equal ...
func conditionsEqual(c1 metav1.Condition, c2 metav1.Condition) (isEqual bool) {
	if c1.Type == c2.Type && c1.Status == c2.Status && c1.Message == c2.Message && c1.Reason == c2.Reason {
		isEqual = true
	}
	return isEqual
}

// AdjustReplicas adjusts the number of Solr replicas to match the spec ...
func (r *SolrCollectionSetReconciler) AdjustReplicas(ctx context.Context,
	collectionSet solrCollectionSet.SolrCollectionSet,
	solrCollections map[string]solr.Collection) (isScaling bool, err error) {

	logger := log.FromContext(ctx)

	logger.Info("checking replicas")

	// Map the spec collections so that the blue/green collections are included ...
	var specCollectionsMap = make(map[string]solrCollectionSet.SolrCollection)
	mapCollections(collectionSet.Spec.Collections, specCollectionsMap, *collectionSet.Spec.BlueGreenEnabled)

	// Iterate the collections defined in the Kube spec and determine what updates need to be made to the replica counts ...
	var adjustReplicas = make(map[string]solr.ReplicationAdjustment)
	for collectionName, _ := range specCollectionsMap {
		// collectionName := collectionSpec.Name
		collection, exists := solrCollections[collectionName]
		if !exists {
			logger.Error(fmt.Errorf("couldn't find collection [%s]", collectionName), "")
		} else {
			adjustment := *collectionSet.Spec.ReplicationFactor - collection.ReplicaCount
			if adjustment != 0 {
				var msg strings.Builder
				msg.WriteString(fmt.Sprintf("collection %s replication factor is %d and replica count is %d",
					collectionName, *collectionSet.Spec.ReplicationFactor, collection.ReplicaCount))

				var action = "add"
				if adjustment < 0 {
					action = "remove"
				}
				msg.WriteString(fmt.Sprintf(" so queueing action to %s %d replicas", action, abs(adjustment)))
				logger.Info(msg.String())

				adjustReplicas[collectionName] = solr.ReplicationAdjustment{
					CurrentCount: collection.ReplicaCount,
					TargetCount:  *collectionSet.Spec.ReplicationFactor,
				}
			}
		}
	}

	for collection, adjustment := range adjustReplicas {
		var diff = adjustment.TargetCount - adjustment.CurrentCount
		if diff > 0 {
			isScaling, err := solrClient.AddReplicas(collection, diff)
			if err != nil {
				return false, err
			}
			if isScaling {
				// Don't error if scaling is happening ...
				return true, nil
			}
		} else {
			err := solrClient.RemoveReplicas(collection, abs(diff))
			if err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

// ManageConfigSets manages Solr schema config sets ....
func (r *SolrCollectionSetReconciler) ManageConfigSets(ctx context.Context, solrCollectionSet solrCollectionSet.SolrCollectionSet,
	checksumCollectionName string) error {

	logger := log.FromContext(ctx)

	logger.Info("checking config sets")

	// Get the config sets from the Solr cluster ...
	var solrConfigSets, err = solrClient.GetConfigSets()
	if err != nil {
		return err
	}
	// Read the Kubernetes configmaps which contain the Solr config sets (aka schemas) ...
	configMapList := &corev1.ConfigMapList{}
	// label selection criteria ...
	selectorLabels := make(map[string]string)
	selectorLabels["collectionSet"] = solrCollectionSet.Name
	selector := labels.SelectorFromSet(selectorLabels)
	listOps := &client.ListOptions{
		Namespace:     solrCollectionSet.Namespace,
		LabelSelector: selector,
	}
	if err := r.List(ctx, configMapList, listOps); err != nil {
		return fmt.Errorf("error listing config maps", err)
	}
	// Map the configmaps that came from Kubernetes by the collection name label ...
	configMaps := map[string]corev1.ConfigMap{}
	for _, cm := range configMapList.Items {
		var name, exists = cm.ObjectMeta.Labels["collection"]
		if !exists {
			return fmt.Errorf("config set configmap %s has no 'collection' label", cm.Name)
		}
		configMaps[name] = cm
	}

	// Grab the config set checksums from Solr to determine whether they have changed.
	// If this is the early in the management process then there may not be any in Solr as they get created when the
	// config set is created (obviously?)...
	checksumsResponse, err := solrClient.Query(checksumCollectionName, "*:*")
	if err != nil {
		return err
	}
	var configSetChecksums = make(map[string]string)
	for _, record := range checksumsResponse {
		var collection = record["collection"]
		var checksum = record["checksum"]
		configSetChecksums[collection.(string)] = checksum.(string)
	}

	// Iterate through the config maps and determine what actions need to be taken to bring Solr in line with the
	// Kubernetes spec ...
	var configMapsToUpload = map[string]corev1.ConfigMap{}
	var configMapsToRemove = map[string]string{} // this doesn't strictly have to be a map, but it's a little easier

	for name, configMap := range configMaps {
		exists := contains(solrConfigSets, name)
		if !exists {
			logger.Info(fmt.Sprintf("queueing config set [%s] for create", name))
			configMapsToUpload[name] = configMap
		} else {
			// compare spec checksum to Solr checksum ....
			var configSetSpec = configMaps[name]
			var specChecksum = checksum(configSetSpec.Data["configset"])
			var solrChecksum, exists = configSetChecksums[name]
			var addToUpdate = false
			if !exists {
				logger.Info(fmt.Sprintf("no checksum found for config set %s in Solr", name))
				addToUpdate = true
			} else {
				// If the checksums differ then flag for update ...
				if specChecksum != solrChecksum {
					addToUpdate = true
				}
			}
			if addToUpdate {
				logger.Info(fmt.Sprintf("queueing config set %s for update", name))
				configMapsToUpload[name] = configMap
			}
		}
	}

	// If cleanup is enabled iterate through the Solr config sets and flag the ones for delete which aren't in the spec
	// (except the ones that are defined outside the Kubernetes spec i.e. are prefixed with "_")
	if *solrCollectionSet.Spec.CleanupEnabled {
		for _, name := range solrConfigSets {
			_, exists := configMaps[name]
			if !exists && !strings.HasPrefix(name, "_") {
				configMapsToRemove[name] = name
			}
		}
	}

	// Process uploads ...
	for collection, configMap := range configMapsToUpload {
		configsetEncoded := configMap.Data["configset"]
		configsetDecoded, err := base64.StdEncoding.DecodeString(configsetEncoded)
		if err != nil {
			return fmt.Errorf("could not base64 decode 'configset' property on configmap %s for collection %s", configMap.Name, collection)
		}
		err = solrClient.UploadConfigSet(collection, configsetDecoded)
		if err != nil {
			return fmt.Errorf("could not upload configset %s", collection)
		}
		// Write the checksum to Solr ...
		var record = fmt.Sprintf(`{
			"collection": "%s",
			"checksum": "%s"
		}`, collection, checksum(configsetEncoded))
		err = solrClient.WriteRecord(checksumCollectionName, record)
		if err != nil {
			return fmt.Errorf("could not write checksum to %s for collection %s", checksumCollectionName, collection)
		}
	}

	// Process removes ...
	for name := range configMapsToRemove {
		err := solrClient.DeleteConfigSet(name)
		if err != nil {
			return fmt.Errorf("could not clean up config set [%s]", name)
		}
	}

	return nil
}

// ManageCollections manages collections ...
func (r *SolrCollectionSetReconciler) ManageCollections(ctx context.Context,
	collectionSet solrCollectionSet.SolrCollectionSet, solrCollections map[string]solr.Collection,
	aliases map[string]string) (changed bool) {

	logger := log.FromContext(ctx)

	logger.Info("checking collections")

	// Read spec data into variables for code readability ...
	replicationFactor := collectionSet.Spec.ReplicationFactor
	isBlueGreenEnabled := collectionSet.Spec.BlueGreenEnabled
	isCleanupEnabled := collectionSet.Spec.CleanupEnabled

	// Reverse map the aliases map (collection->aliases). This is used down in the delete collection section ...
	var collectionsToAliasesMap = make(map[string]string)
	for alias, collection := range aliases {
		collectionsToAliasesMap[collection] = alias
	}

	// Determine which collections need to be created.
	// Map the collections collectionSet for easy access
	// Create _blue/_green entries if isBlueGreenEnabled is true. Otherwise, just use the plain collection name.
	var specCollectionsMap = make(map[string]solrCollectionSet.SolrCollection)
	mapCollections(collectionSet.Spec.Collections, specCollectionsMap, *isBlueGreenEnabled)

	// maps of collection actions to take ...
	var createCollectionsMap = make(map[string]solrCollectionSet.SolrCollection)
	var deleteAliasesMap = make(map[string]string)
	var deleteCollectionsMap = make(map[string]solrCollectionSet.SolrCollection)
	var adjustReplicationFactorMap = make(map[string]solr.Collection)

	// Iterate through the specs and see if the collection exists in Solr. If not add it to the "create" map ...
	for collectionName, spec := range specCollectionsMap {
		_, exists := solrCollections[collectionName]
		if !exists {
			logger.Info(fmt.Sprintf("queueing collection [%s] for create", collectionName))
			createCollectionsMap[collectionName] = spec
		}
	}

	// If cleanup is enabled, iterate though the solrCollections collections and see if they are still specified.
	// If not add to the "delete" map assuming clean up is enabled
	if *isCleanupEnabled {
		for collectionName, _ := range solrCollections {
			// if the collection is no longer in the spec then queue for removal (as long as it isn't prefixed with "_") ...
			spec, exists := specCollectionsMap[collectionName]
			if !exists && !strings.HasPrefix(collectionName, "_") {
				logger.Info(fmt.Sprintf("queueing collection [%s] for removal", collectionName))
				deleteCollectionsMap[collectionName] = spec
				// Check for an alias as that'll have to be cleaned up before the collection can be removed ...
				alias, exists := collectionsToAliasesMap[collectionName]
				if exists {
					logger.Info(fmt.Sprintf("queueing alias [%s] for removal", alias))
					deleteAliasesMap[collectionName] = alias
				}
			}
		}
	}

	// Iterate though the solrCollections/existing collections and see if the replication factor needs updating.
	// (collection that haven't been created yet will automatically get created with the current replication factor)
	for collectionName, collection := range solrCollections {
		// make sure the collection is part of the collectionSet (and isn't being cleaned up or ignored)
		_, exists := specCollectionsMap[collectionName]
		if exists {
			if collection.ReplicationFactor != *replicationFactor {
				logger.Info(fmt.Sprintf("queueing collection [%s] for replication factor adjustment", collectionName))
				adjustReplicationFactorMap[collectionName] = collection
			}
		}
	}

	// Process create collections ...
	if len(createCollectionsMap) > 0 {
		logger.Info("creating collections", "collections", seqToString(maps.Keys(createCollectionsMap)))
		for collectionName, collectionSpec := range createCollectionsMap {
			err := solrClient.CreateCollection(collectionName, collectionSpec.ConfigsetName, *collectionSet.Spec.ReplicationFactor)
			if err != nil {
				logger.Error(err, "create collection failed")
			}
			// If this is a blue/green then go ahead and create an alias if one doesn't already exist ...
			if *isBlueGreenEnabled {
				_, exists := aliases[collectionSpec.Alias]
				if !exists {
					err = solrClient.AssignAlias(collectionSpec.Alias, collectionName)
					if err != nil {
						logger.Error(err, "create alias failed")
					}
				}
			}
		}
		changed = true
	}

	// Process delete aliases ...
	if len(deleteAliasesMap) > 0 {
		logger.Info("deleting aliases", "aliases", seqToString(maps.Keys(deleteAliasesMap)))
		for alias, _ := range deleteAliasesMap {
			err := solrClient.DeleteAlias(alias)
			if err != nil {
				logger.Error(err, fmt.Sprintf("delete alias [%s] failed", alias))
			}
		}
		changed = true
	}

	// Process delete collections ...
	if len(deleteCollectionsMap) > 0 {
		logger.Info("deleting collections", "collections", seqToString(maps.Keys(deleteCollectionsMap)))
		for collectionName, _ := range deleteCollectionsMap {
			err := solrClient.DeleteCollection(collectionName)
			if err != nil {
				logger.Error(err, fmt.Sprintf("delete collection [%s] failed", collectionName))
			}
		}
		changed = true
	}

	// Process adjust replication factor ...
	if len(adjustReplicationFactorMap) > 0 {
		logger.Info("adjusting replication factor", "collections", seqToString(maps.Keys(deleteCollectionsMap)))
		for collectionName, _ := range adjustReplicationFactorMap {
			err := solrClient.SetReplicationFactor(collectionName, *replicationFactor)
			if err != nil {
				logger.Error(err, "replication factor update on failed")
			}
		}
		changed = true
	}

	return changed
}

// makeSolrClient Creates a client for the Solr API ...
func (r *SolrCollectionSetReconciler) makeSolrClient(ctx context.Context, secretRef string, clusterUrl string) (solrClient solr.SolrClient, error error) {
	// Query Solr for the actual cluster state ...
	if secretRef != "" {

		basicAuthSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      secretRef,
			Namespace: "default",
		}, basicAuthSecret)
		if err != nil {
			return solrClient, fmt.Errorf("could not read the basic auth secret [%s]", secretRef)
		}
		// Initialize solrClient if it isn't already ...
		if solrClient == (solr.SolrClient{}) {
			solrClient = solr.SolrClient{
				Username: string(basicAuthSecret.Data["username"]),
				Password: string(basicAuthSecret.Data["password"]),
				Url:      clusterUrl,
			}
		}
	} else {
		return solrClient, fmt.Errorf("no secret was provided for Solr basic auth")
	}
	return solrClient, nil
}

// checksum calculates the md5 checksum of a string.
func checksum(data string) string {
	bytes := []byte(data)
	hash := md5.Sum(bytes)
	return hex.EncodeToString(hash[:])
}

// seqToString Takes a sequence and turns it into a string where the elements are comma delimited
func seqToString(seq iter.Seq[string]) string {
	var parts []string
	for v := range seq {
		parts = append(parts, v)
	}
	return strings.Join(parts, ", ")
}

// createChecksumCollection creates a checksum config set and collection ...
func createChecksumCollection(checksumsCollectionName string, replicationFactor int32) error {
	// assume if the collection doesn't exist then the schema doesn't either, so create it ...
	bytes, err := utils.Zip("checksum_collection_configset", checksumCollectionSchema)
	if err != nil {
		return err
	}
	err = solrClient.UploadConfigSet(configChecksumsConfigSetName, bytes)
	if err != nil {
		return err
	}
	// create the collection
	err = solrClient.CreateCollection(checksumsCollectionName, configChecksumsConfigSetName, replicationFactor)
	if err != nil {
		return err
	}
	return nil
}

// mapCollections maps collection to their collection name ...
func mapCollections(specCollections []solrCollectionSet.SolrCollection,
	storage map[string]solrCollectionSet.SolrCollection, isBlueGreenEneabled bool) {
	// Map the collections collectionsSpec for easy access
	// Create _blue/_green entries if isBlueGreenEnabled is true. Otherwise, just use the plain collection name.

	for _, spec := range specCollections {
		collectionName := spec.Name
		if isBlueGreenEneabled {
			storage[collectionName+"_blue"] = spec
			storage[collectionName+"_green"] = spec
		} else {
			storage[collectionName] = spec
		}
	}
}

// RequeueOnError handles reconcile errors ...
func (r *SolrCollectionSetReconciler) RequeueOnError(
	ctx context.Context,
	req ctrl.Request,
	collectionSet *solrCollectionSet.SolrCollectionSet,
	error error) (ctrl.Result, error) {

	logger := log.FromContext(ctx)
	logger.Info("requeueing on error")

	// Because an error has been hit, the collection set is no longer stable ...
	stableCondition := metav1.Condition{
		Type:    typeSolrCollectionSetStable,
		Status:  metav1.ConditionFalse,
		Reason:  reasonSolrCollectionSetReconcileError,
		Message: error.Error(),
	}

	// If the new status object and the old status object differ, then apply the changes ...
	oldInstance := collectionSet.DeepCopy()
	// Make a copy of the
	statusCopy := oldInstance.Status.DeepCopy()
	// Write the conditions into the status object ...
	meta.SetStatusCondition(&statusCopy.Conditions, stableCondition)

	// If anything changed then write out the new status. This will cause a call to Reconcile() to be queued for
	// immediate processing.
	if !reflect.DeepEqual(collectionSet.Status, *statusCopy) {
		collectionSet.Status = *statusCopy
		err := r.Status().Patch(ctx, collectionSet, client.MergeFrom(oldInstance))
		if err != nil {
			logger.Error(err, fmt.Sprintf("failed to save collection set status [%s]", collectionSet.Name))
			return requeueWithBackoff()
		}
		// Re-fetch the SolrCollectionSet after updating the status
		if err := r.Get(ctx, req.NamespacedName, collectionSet); err != nil {
			logger.Error(err, fmt.Sprintf("failed to re-fetch SolrCollectionSet [%s]", collectionSet.Name))
			return requeueWithBackoff()
		}
	}

	return requeue()
}

// requeue returns a standard delayed requeue ...
func requeue() (ctrl.Result, error) {
	// return reconcile.Result{RequeueAfter: time.Second * errorRequeueSeconds}, nil
	//return requeueImmediately()
	return reconcile.Result{}, nil
}

// requeueImmediately does just that ...
func requeueImmediately() (ctrl.Result, error) {
	return reconcile.Result{RequeueAfter: time.Millisecond}, nil
}

// requeueImmediately does just that ...
func requeueWithBackoff() (ctrl.Result, error) {
	return reconcile.Result{RequeueAfter: time.Second * backoffRequeueSeconds}, nil
}

// abs calculates the absolute value of an int32 ...
func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

// contains tests if the given list contains the given string ...
func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// countSolrCollections counts up the number of collections in the given map MINUS the unmanaged ones ...
func countSolrCollections(collections map[string]solr.Collection) (count int) {
	for _, collection := range collections {
		// Don't count collections that begin with _ ...
		if !strings.HasPrefix(collection.Name, "_") {
			count++
		}
	}
	return count
}

// countSpecifiedCollections counts the number of specified collections taking into account blue/green collections
func countSpecifiedCollections(collections []solrCollectionSet.SolrCollection, isBlueGreenEnabled bool) (count int) {
	multiplier := 1
	count = len(collections)
	if isBlueGreenEnabled {
		multiplier = 2
	}
	return count * multiplier
}

// SetupWithManager sets up the controller with the Manager.
func (r *SolrCollectionSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&solrCollectionSet.SolrCollectionSet{}).Named("solrcollectionset").Complete(r)
}
