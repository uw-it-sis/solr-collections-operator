# solr-collections-operator
The Solr Collections operator manages Solr collections and replicas of those collections.

## Description
The Solr Collections operator handles creating Solr collections by defining how the collection should look in a CRD.
The operator handles ...
* Creating and/or removing collections from the Solr cluster
* Managing the replication factor of collections
* Adding or removing replicas 
* Managing Solr config sets (aka collection schemas)

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster

NOTES:
* The project was setup with kubebuilder like ...
  * kubebuilder init --domain example.com --repo github.com/your-username/my-operator
  * kubebuilder create api --group mygroup --version v1 --kind MyResource
  * The docs for kubebuilder are here ... https://book.kubebuilder.io/
* The project uses a Makefile for various things
  * 
* The operator can run locally. It will read your current kubectl config to talk to a cluster. SO BE CAREFUL.
  * You'll need a tunnel to the solr cluster and that'll require a localhost address in the CRD
* If you update the CRD defs they have to deployed to the cluster like ...
  * make manifests
  * kubectl apply -f config/crd/bases/solrcollections.solr.sis.uw.edu_solrcollectionsets.yaml
* kubebuilder is provided by mise
Helm ...
* make build-installer IMG=ghcr.io/uw-it-sis/solr-collections-operator/solr-collections-operator:v1.0.0
  * This creates dist/install.yaml which contains the Kube resources to install the operator with ...
    * kubectl apply -f https://raw.githubusercontent.com/<org>/project-v4/<tag-or-branch>/dist/install.yaml
    * Note: I've never installed this way ^^^, but it's the default/built-in way
* Create Helm chart from kustomize output either to the default location or a custom location ...
  * kubebuilder edit --plugins=helm/v2-alpha
  * kubebuilder edit --plugins=helm/v2-alpha --output-dir=charts
    * This creates the charts directory at the top level
* How to overwrite preserved files if needed
  * kubebuilder edit --plugins=helm/v2-alpha --force --output-dir=charts



make help

FIXME: Nothing below here has been reviewed/updated to fit our situation 

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

