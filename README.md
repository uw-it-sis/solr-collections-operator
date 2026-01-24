# solr-collections-operator
The Solr Collections operator manages Solr collections and replicas of those collections.

## Description
The Solr Collections operator handles creating Solr collections by defining how the collection should look in a CRD.
The operator handles ...
* Creating and/or removing collections from the Solr cluster
* Managing the replication factor of collections
* Adding or removing replicas 
* Managing Solr config sets (aka collection schemas)

## Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

The kubebuilder can be provided by mise

## Overview

### The Operator Pattern
Basically, operators are long-running apps that manage a Kubernetes resource of some sort. 
For example the autoscaling operator manages worker nodes, the Solr Operator manages Solr cluster
deployaments, the Solr Collections operator (this app) manages the collections that are deployed 
on a Solr cluster.

The primary building block for an operator is the Custom Resource Definition (CRD). The CRD is
effectively the schema for Kubernetes resources (manifests) which hold the desired state of the resource the
operator will manage. The main resource for this operator is SolrCollectionSet. An instance of
SolrCollectionSet defines the collections you would like to have in the Solr cluster.

The operators job is to listen for changes to CRD instances. When changes occur to the CRD instance
the operator attempts to bring the state of the managed resource in line with the state of the CRD
instance.

See Kube Commands for info on how to see operator related resources in Kuberntes

### Project Setup
This project was initiated with Kubebuilder by going ...
```sh
kubebuilder init --domain solr.sis.uw.edu --repo github.com/uw-it-sis/solr-collections-operator
kubebuilder create api --group solrcollections --version v1 --kind SolrCollectionSet
```
The docs for Kubebuilder are at https://book.kubebuilder.io/

### Make
The project leans heavily on Make for common operators. 
Make's targets are defined in file, Makefile. Most of the stuff in there came from Kubebuilder, but
I also added some targets.

### Generated Code
This project also embraces the concept of generated code. CRDs, the install.yaml file, the Helm chart, and other stuff
is actually generated from a curious mixture of Go structs and annotations (which Kubebuilder refs to as "markers").
Often when you change something you'll have to run a make target to have the generated code updated.

### Run/Change/Test

To work out the operator logic the app can run locally with ...

    make run

IntelliJ also knows how to run Go programs. 

However, the operator will connect to whichever Kubernetes cluster is refereneced in `~/.kube/config`, 
so there's a fair amount of potential for destruction. **So, do be careful.**

You'll also need to establish a tunnel to the Solr cluster you want to run against like ...

    ssh -i ~/.ssh/aws_rsa shell.planning-dev.sis.uw.edu -L8983:solr.planning-dev.local.net:8983

### Custom Resource Definitions (CRD)

In the project the CRDs are defined in `api/v1/solrcollectionset_types.go`.

CRDs are composed of Go struts with markers for validations and other functionality 
See https://book.kubebuilder.io/reference/markers/crd-validation.html

An important thing to understand is that if you change the markers or the types or anything else related to the CRD 
the generated code will need to updated as well by going ...

    make manifests
... or ....

    make build (which in turn calls make manifests)

This will update the file `config/crd/bases/solrcollections.solr.sis.uw.edu_solrcollectionsets.yaml`.

It's also important to know that if you are running locally the CRDs have to be installed on the Kubernetes cluster by 
going ...

    kubectl apply -f config/crd/bases/solrcollections.solr.sis.uw.edu_solrcollectionsets.yaml

The Helm chart automatically installs the CRDs, so this step is only necessary if you are trying to run locally and
the operator Helm chart isn't installed. However, if you install the CRDs manually and then try to install the Helm
chart the chart will fail because the manually installed CRDs are in the way. To overcome this issue you can manually
delete the CRDs with ...

    kubectl delete crd solrcollectionsets.solrcollections.solr.sis.uw.edu

Note that deleting the CRDs will also delete any instances of the CRD, so yea. That happens.

Also, not that deleting the Helm chart will remove the CRDs, manifests, and the operator itself. However, this won't 
affect the Solr cluster or the collections themselves.

### The Helm Chart

The Kubebuilder Helm chart plugin generates artifacts based on the contents of `dist/install.yaml`

To manually generate `dist/install.yaml` you can go ...

    make build-installer IMG=ghcr.io/uw-it-sis/solr-collections-operator/solr-collections-operator:v1.0.0

... where v1.0.0 is the version of the project. 

To manually update the Helm chart once `dist/install.yaml` has been updated you can go ...

    kubebuilder edit --plugins=helm/v2-alpha --output-dir=charts

Chart.yaml is never overwritten. The files `values.yaml`, `_helpers.tpl`, `.helmignore`, and 
`.github/workflows/test-chart.yml` are preserved, meaning that they won't be overwritten unless you go ...

    kubebuilder edit --plugins=helm/v2-alpha --force --output-dir=charts

See https://kubebuilder.io/plugins/available/helm-v2-alpha for additional info about the Helm chart plugin.

The Helm chart is published as a Github Pages page paired with a release this is accomplished with the 
chart-releaser action (https://github.com/helm/chart-releaser-action).

To get Pages to work I had to create an empty gn-pages branch. Also, had to make the repo and the page public.

## Devving

### Deploy changes to Helm chart 


### Useful Kube Commands

You can see what CRDs are installed in a Kubernetes cluster by going ...

    kubectl get crds

To see details of a CRD you can go ...

    kubectl describe crd <crd.name>
    kubectl describe solrcollectionsets.solrcollections.solr.sis.uw.edu

To see the details of a CRD prop by prop you can go ...

    kubectl explain <crd.prop.subprop>
    kubectl explain solrcollectionsets.solrcollections.solr.sis.uw.edu
    kubectl explain solrcollectionsets.solrcollections.solr.sis.uw.edu.spec

To see the operator pod you can go ...

    kubectl get pods

To see what the operator is upto you can go ... 

    kubectl logs -f <pod-name>


NOTES:


* To see what Make targets are available go ... 
  *  make help

FIXME: Nothing below here has been reviewed/updated to fit our situation 

### To Deploy on the cluster (Note: We don't do it this way. Instead we create a Helm chart and deploy that with Terraform)

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/solr-collections-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/solr-collections-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/solr-collections-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/solr-collections-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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

