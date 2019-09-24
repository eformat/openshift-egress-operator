# OpenShift Egress Operator

## Build Image

Build locally
```
make docker-build
```

Push to quay.io
```
make docker-push
```

## Deploying the Operator

This is a cluster-level operator that you can deploy in any namespace, `microsegmentation-operator` is recommended.

```shell
oc new-project microsegmentation-operator
```

Deploy the cluster resources. Given that a number of elevated permissions are required to resources at a cluster scope the account you are currently logged in must have elevated rights.

```shell
oc apply -f deploy/
```

## Run Locally

```
operator-sdk up local --namespace "test" --verbose
```

## Configuring Operator Using Annotations

The egress operator allows you to create `Netnamespace` and `HostSubnet` resources using `Namespace` annotations.

This feature is activated by this annotation: `microsegmentation-operator.redhat-cop.io/microsegmentation: "true"`.

```
oc annotate namespace test microsegmentation-operator.redhat-cop.io/microsegmentation='true'
```

To set an `EgressIP` address that may be automatically assigned to a set of `EgressHosts` for any given namespace - use the following annotations.

| Annotation  | Description  |
| - | - |
| `microsegmentation-operator.redhat-cop.io/egress-ip`  | An egress ip address on the hosts subnet e.g. `192.168.5.150`  |
| `microsegmentation-operator.redhat-cop.io/egress-cidr` | The CIDR range for the egress-ip e.g. `192.168.5.0/24`  |
| `microsegmentation-operator.redhat-cop.io/egress-hosts`  | A comma separated list of nodes for the egress vip (more than one required for HA) e.g. `infra0,infra1` |

```
oc annotate namespace test \
    microsegmentation-operator.redhat-cop.io/egress-ip=192.168.5.150 \
    microsegmentation-operator.redhat-cop.io/egress-cidr=192.168.5.0/24 \
    microsegmentation-operator.redhat-cop.io/egress-hosts=infra0,infra1 --overwrite
```
