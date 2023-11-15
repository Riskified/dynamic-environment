# DynamicEnv CRD Reference

## Packages
- [riskified.com/v1alpha1](#riskifiedcomv1alpha1)


## riskified.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the riskified v1alpha1 API group

### Resource Types
- [DynamicEnv](#dynamicenv)



#### ConsumerStatus





_Appears in:_
- [DynamicEnvStatus](#dynamicenvstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | The name of the resource |
| `namespace` _string_ | The namespace where the resource is created |
| `status` _LifeCycleStatus_ | The life cycle status of the resource |
| `hash` _integer_ | Hash of the current consumer - for internal use |
| `errors` _[StatusError](#statuserror) array_ | List of errors related to the consumer |


#### ContainerOverrides



Defines the details of the container on which changes need to be made and the relevant overrides

_Appears in:_
- [Subset](#subset)

| Field | Description |
| --- | --- |
| `containerName` _string_ | Container name to override in multiple containers' environment. If not specified we will use the first container. |
| `image` _string_ | Docker image name overridden to the desired subset The Docker image found in the original deployment is used if this is not provided. |
| `command` _string array_ | Entrypoint array overridden to the desired subset The docker image's ENTRYPOINT is used if this is not provided. |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#envvar-v1-core) array_ | Additional environment variable to the given deployment |


#### DynamicEnv



DynamicEnv is the Schema for the dynamicenvs API



| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `riskified.com/v1alpha1` |
| `kind` _string_ | `DynamicEnv` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[DynamicEnvSpec](#dynamicenvspec)_ |  |
| `status` _[DynamicEnvStatus](#dynamicenvstatus)_ |  |


#### DynamicEnvSpec



DynamicEnvSpec defines the desired state of DynamicEnv

_Appears in:_
- [DynamicEnv](#dynamicenv)

| Field | Description |
| --- | --- |
| `istioMatches` _[IstioMatch](#istiomatch) array_ | A list of matchers (partly corresponds to IstioMatch). Each match will have a rule of its own (merged with existing rules) ordered by their order here. |
| `subsets` _[Subset](#subset) array_ | Who should participate in the given dynamic environment |
| `consumers` _[Subset](#subset) array_ | Consumers are like subsets but for deployments that do not open a service but connect to external resources for their work (e.g, offline workers). They are equivalent to subsets in the sense that they launch overriding deployments with custom image and/or settings. However, since they are only consumers no virtual service or destination route will be pointing to them. |


#### DynamicEnvStatus



DynamicEnvStatus defines the observed state of DynamicEnv

_Appears in:_
- [DynamicEnv](#dynamicenv)

| Field | Description |
| --- | --- |
| `subsetsStatus` _object (keys:string, values:[SubsetStatus](#subsetstatus))_ |  |
| `consumersStatus` _object (keys:string, values:[ConsumerStatus](#consumerstatus))_ |  |
| `state` _GlobalReadyStatus_ |  |
| `totalCount` _integer_ | desired subsets and consumers count |
| `totalReady` _integer_ | number of available subsets and consumers |


#### IstioMatch



specifies a set of criterion to be met in order for the rule to be applied to the HTTP request This field is immutable after creation.

_Appears in:_
- [DynamicEnvSpec](#dynamicenvspec)

| Field | Description |
| --- | --- |
| `headers` _object (keys:string, values:[StringMatch](#stringmatch))_ | Header values are case-sensitive and formatted as follows:<br/> - `exact: "value"` for exact string match<br/> - `prefix: "value"` for prefix-based match<br/> - `regex: "value"` for RE2 style regex-based match (https://github.com/google/re2/wiki/Syntax). |
| `sourceLabels` _object (keys:string, values:string)_ | One or more labels that constrain the applicability of a rule to source (client) workloads with the given labels. |


#### ResourceStatus



ResourceStatus shows the status of each item created/edited by DynamicEnv

_Appears in:_
- [ConsumerStatus](#consumerstatus)
- [SubsetStatus](#subsetstatus)

| Field | Description |
| --- | --- |
| `name` _string_ | The name of the resource |
| `namespace` _string_ | The namespace where the resource is created |
| `status` _LifeCycleStatus_ | The life cycle status of the resource |


#### StatusError



StatusError shows an error we want to display in the status with the last time it happened. This *does not* have to be the only time it happened. The idea is that a list of errors should only contain single occurrence of an error (just the last).

_Appears in:_
- [ConsumerStatus](#consumerstatus)
- [SubsetErrors](#subseterrors)



#### StringMatch



Describes how to match a given string in HTTP headers. Match is case-sensitive. one and only one of the fields needs to be defined (oneof)

_Appears in:_
- [IstioMatch](#istiomatch)

| Field | Description |
| --- | --- |
| `exact` _string_ |  |
| `prefix` _string_ |  |
| `regex` _string_ |  |


#### Subset



Subsets defines how to generate subsets from existing Deployments

_Appears in:_
- [DynamicEnvSpec](#dynamicenvspec)

| Field | Description |
| --- | --- |
| `name` _string_ | Deployment name (without namespace) |
| `namespace` _string_ | Namespace where the deployment is deployed |
| `podLabels` _object (keys:string, values:string)_ | Labels to add to the pods of the deployment launched by this subset. Could be used in conjunction with 'SourceLabels' in the `IstioMatches`. |
| `replicas` _integer_ | Number of deployment replicas. Default is 1. Note: 0 is *invalid*. |
| `containers` _[ContainerOverrides](#containeroverrides) array_ | A list of container overrides (at least one of Containers or InitContainers must not be empty) |
| `initContainers` _[ContainerOverrides](#containeroverrides) array_ | A list of init container overrides (at least one of Containers or InitContainers must not be empty) |
| `defaultVersion` _string_ | Default version for this subset (if different then the global default version). This is the version that will get the default route. |


#### SubsetErrors



SubsetErrors contains all global errors related to set subset.

_Appears in:_
- [SubsetStatus](#subsetstatus)

| Field | Description |
| --- | --- |
| `deployment` _[StatusError](#statuserror) array_ | Subset's deployment global errors. |
| `destinationRule` _[StatusError](#statuserror) array_ | Subset's destination-rule global errors. |
| `virtualServices` _[StatusError](#statuserror) array_ | Subset's virtual-services global errors. |
| `subset` _[StatusError](#statuserror) array_ | Errors related to subset but not to any of the launched resources |




#### SubsetStatus



SubsetStatus Contains aggregation of all resources status connected to set subset.

_Appears in:_
- [DynamicEnvStatus](#dynamicenvstatus)

| Field | Description |
| --- | --- |
| `deployment` _[ResourceStatus](#resourcestatus)_ | Status of the deployment that belongs to the subset |
| `destinationRules` _[ResourceStatus](#resourcestatus) array_ | Status of the destination-rule that belongs to the subset |
| `virtualServices` _[ResourceStatus](#resourcestatus) array_ | Status of the virtual-service that belongs to the subset |
| `subsetErrors` _[SubsetErrors](#subseterrors)_ | A list of global errors related to subset resources |
| `hash` _integer_ | Hash of the current subset - for internal use |


