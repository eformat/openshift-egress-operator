## openshift-egress-operator

### Run Locally

```
operator-sdk up local --namespace "test" --verbose
```

### Deploy to OpenShift

```
oc new-project microsegmentation-operator
oc apply -f deploy/
```