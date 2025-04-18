# Resource Tracking

## Tracking Kubernetes resources by annotation

Argo CD can be instructed to use the following methods for tracking:

1. `annotation` (default) - Argo CD uses the `argocd.argoproj.io/tracking-id` annotation to track application resources. Use this when you don't need to maintain both the label and the annotation.
1. `annotation+label` - Argo CD uses the `app.kubernetes.io/instance` label but only for informational purposes. The label is not used for tracking purposes, and the value is still truncated if longer than 63 characters. The annotation `argocd.argoproj.io/tracking-id` is used instead to track application resources. Use this for resources that you manage with Argo CD, but still need compatibility with other tools that require the instance label.
1. `label` - Argo CD uses the `app.kubernetes.io/instance` label


Here is an example of using the annotation method for tracking resources:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deployment
  namespace: default
  annotations:
    argocd.argoproj.io/tracking-id: my-app:apps/Deployment:default/my-deployment
```

The advantages of using the tracking id annotation is that there are no clashes any
more with other Kubernetes tools and Argo CD is never confused about the owner of a resource. The `annotation+label` can also be used if you want other tools to understand resources managed by Argo CD.

### Installation ID

If you are managing one cluster using multiple Argo CD instances, you will need to set `installationID` in the Argo CD ConfigMap. This will prevent conflicts between
the different Argo CD instances:

* Each managed resource will have the annotation `argocd.argoproj.io/installation-id: <installation-id>`
* It is possible to have applications with the same name in Argo CD instances without causing conflicts.

### Non self-referencing annotations
When using the tracking method `annotation` or `annotation+label`, Argo CD will consider the resource properties in the annotation (name, namespace, group and kind) to determine whether the resource should be compared against the desired state. If the tracking annotation does not reference the resource it is applied to, the resource will neither affect the application's sync status nor be marked for pruning.

This allows other kubernetes tools (e.g. [HNC](https://github.com/kubernetes-sigs/hierarchical-namespaces)) to copy a resource to a different namespace without impacting the Argo CD application's sync status. Copied resources will be visible on the UI at top level. They will have no sync status and won't impact the application's sync status.


## Tracking Kubernetes resources by label

In this mode, Argo CD identifies resources it manages by setting the application instance label to the name of the managing Application on all resources that are managed (i.e. reconciled from Git). The default label used is the well-known label `app.kubernetes.io/instance`.

Example:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deployment
  namespace: default
  labels:
    app.kubernetes.io/instance: some-application
```

This approach works ok in most cases, as the name of the label is standardized and can be understood by other tools in the Kubernetes ecosystem.

There are however several limitations:

* Labels are truncated to 63 characters. Depending on the size of the label you might want to store a longer name for your application
* Other external tools might write/append to this label and create conflicts with Argo CD. For example several Helm charts and operators also use this label for generated manifests confusing Argo CD about the owner of the application
* You might want to deploy more than one Argo CD instance on the same cluster (with cluster wide privileges) and have an easy way to identify which resource is managed by which instance of Argo CD

### Use custom label

Instead of using the default `app.kubernetes.io/instance` label for resource tracking, Argo CD can be configured to use a custom label. Below example sets the resource tracking label to `argocd.argoproj.io/instance`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  labels:
    app.kubernetes.io/name: argocd-cm
    app.kubernetes.io/part-of: argocd
data:
  application.instanceLabelKey: argocd.argoproj.io/instance
```

## Choosing a tracking method

To actually select your preferred tracking method edit the `resourceTrackingMethod` value contained inside the `argocd-cm` configmap.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  labels:
    app.kubernetes.io/name: argocd-cm
    app.kubernetes.io/part-of: argocd
data:
  application.resourceTrackingMethod: annotation
```
Possible values are `label`, `annotation+label` and `annotation` as described above.

Note that once you change the value you need to sync your applications again (or wait for the sync mechanism to kick-in) in order to apply your changes.

You can revert to a previous choice, by changing the configmap again.
