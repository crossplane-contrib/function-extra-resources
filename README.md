# function-extra-resources
[![CI](https://github.com/crossplane/function-template-go/actions/workflows/ci.yml/badge.svg)](https://github.com/crossplane/function-template-go/actions/workflows/ci.yml)

A function for selecting extra resources via [composition function][functions]s in [Go][go].

## Using `function-extra-resources`

Please see the example in `./examples`

`function-extra-resources` is generally most useful in tandem with a function that can reference the many resources like
`function-go-templating`.

### Creating objects from other's found in the local cluster.
``` yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: function-environment-configs
spec:
  compositeTypeRef:
    apiVersion: example.crossplane.io/v1
    kind: XR
  mode: Pipeline
  pipeline:
  - step: pull-extra-resources
    functionRef:
      name: function-extra-resources
    input:
      apiVersion: extra-resources.fn.crossplane.io/v1beta1
      kind: Input
      spec:
        extraResources:
          - kind: XCluster
            into: XCluster
            apiVersion: example.crossplane.io/v1
            type: Selector
            selector:
              maxMatch: 2
              minMatch: 1
              matchLabels:
                - key: type
                  type: Value
                  value: cluster
  - step: go-templating
    functionRef:
      name: function-go-templating
    input:
      apiVersion: gotemplating.fn.crossplane.io/v1beta1
      kind: GoTemplate
      source: Inline
      inline:
        template: |
            {{- $XClusters := index (index .context "apiextensions.crossplane.io/extra-resources") "XCluster" }}
            {{- range $i, $A := $XClusters }}
            ---
            apiVersion: vault.upbound.io/v1beta1
            kind: VaultRole
            metadata:
              annotations:
                gotemplating.fn.crossplane.io/composition-resource-name: {{index (index $A "metadata") "name"}}
            spec:
              forProvider:
            {{- end}}
```


## Installing the `function-extra-resources` Function into a Cluster

``` shell
cat <<EOF | kubectl apply -f -
apiVersion: pkg.crossplane.io/v1beta1
kind: Function
metadata:
  name: function-extra-resources
spec:
  package: xpkg.upbound.io/crossplane-contrib/function-extra-resources:latest
EOF
```

## Local dev.

### Air

`air` is not strictly necessary, but helpful.

Installing [air](https://github.com/cosmtrek/air) allows quick iterative local development.

`air` is a live reloader that watches for local file changes.

Once installed, running

`air -- --insecure --debug --address localhost:9443`.

Shoud get the function process/server build and running to serve CLI function requests.

### After locally serving function-extra-resources

`./run.sh` will use the crossplane CLI to run our basic example in `./examples`

### Crossplane Function Basics

This function uses [Go][go], [Docker][docker], and the [Crossplane CLI][cli].

```shell
# Run code generation - see input/generate.go
$ go generate ./...

# Run tests - see fn_test.go
$ go test ./...

# Build the function's runtime image - see Dockerfile
$ docker build . --tag=runtime

# Build a function package - see package/crossplane.yaml
$ crossplane xpkg build -f package --embed-runtime-image=runtime
```

[functions]: https://docs.crossplane.io/latest/concepts/composition-functions
[go]: https://go.dev
[function guide]: https://docs.crossplane.io/knowledge-base/guides/write-a-composition-function-in-go
[package docs]: https://pkg.go.dev/github.com/crossplane/function-sdk-go
[docker]: https://www.docker.com
[cli]: https://docs.crossplane.io/latest/cli
