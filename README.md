<img src="assets/logo.svg" alt="eamerald logo">

# Eamerald - cloud-native authorization for modern applications and APIs

[![Go Report Card](https://goreportcard.com/badge/github.com/threehook/eamerald)](https://goreportcard.com/report/github.com/threehook/eamerald)
[![ci](https://github.com/threehook/eamerald/actions/workflows/ci.yaml/badge.svg)](https://github.com/threehook/eamerald/actions/workflows/ci.yaml)
![Apache 2.0](https://img.shields.io/github/license/threehook/eamerald)
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/threehook/eamerald)

Eamerald is an open-source authorization service providing fine-grained, real-time, policy-based access control for applications and APIs.

It uses the [Open Policy Agent](https://www.openpolicyagent.org/) (OPA) as its decision engine, and provides a built-in directory that is inspired by the Google [Zanzibar](https://research.google/pubs/pub48190/) data model.

Authorization policies can leverage user attributes, group membership, application resources, and relationships between them. All data used for authorization is modeled and stored locally in an embedded database, so authorization decisions can be evaluated quickly and efficiently.

<img src="assets/topaz_model_viz.gif" alt="topaz model visualization">

## Documentation and support

See the [docs](docs/) directory for configuration reference (`docs/config.md`) and feature-flag documentation (`docs/fflag/`).

## Benefits

* **Authorization in one place**: a single authorization service, instead of spreading authorization logic everywhere.
* **Fine-grained**: following the Principle of Least Privilege, assign the smallest set of fine-grained permissions to each user or group.
* **Policy-based**: convert authorization "spaghetti code" into a policy expressed in its own domain-specific language, managed as code, and built into an immutable, signed artifact.
* **Real-time**: gate each protected resource with an authorization call that ensures the user has the right permission.
* **Blazing fast**: deploy the authorizer as a sidecar or microservice, right next to your app, for low latency and high availability.
* **Comprehensive decision logging**: log every decision to facilitate audit trails, compliance, and forensics.
* **Flexible authorization model**: Start simple, and grow from multi-tenant RBAC to ABAC or ReBAC, or a combination - see [authorization models](docs/authorization-models.md).
* **Capture your domain model**: Create object types and relationships that reflect your domain model.
* **Separation of concerns**: application developers can own the app logic, and security engineers can own the authorization policy.

## Table of Contents
- [Getting Eamerald](#getting-eamerald)
    - [Installation](#installation)
    - [Building from source](#building-from-source)
    - [Running with Docker](#running-with-docker)
- [Quickstart](#quickstart)
    - [Install container image](#install-eamerald-authorizer-container-image)
    - [Install Todo template](#install-the-todo-template)
    - [Issue an API call](#issue-an-api-call)
    - [Issue authorization request](#issue-an-authorization-request)
    - [Run the sample application](#run-the-sample-application)
- [Command Line](#command-line-options)
- [gRPC Endpoints](#grpc-endpoints)
- [Demo video](#demo)
- [Credits](#credits)
- [Contribution Guidelines](#contribution-guidelines)

## Getting Eamerald

### Installation

`eamerald` is available for Linux and macOS platforms.

* Binaries for Linux and macOS are available as tarballs in the [release](https://github.com/threehook/eamerald/releases) page.

* Via a GO install

```console
$ go install github.com/threehook/eamerald/mrld@latest
```

### Building from source

`eamerald` requires Go 1.27.x to build (see `Makefile`'s `GO_VER`); `go.mod` is pinned to 1.26.3. In order to build `eamerald` from source you must:

 1. Clone the repo
 2. Build and run the executable

```console
$ make build
$ ./dist/mrld_<os>_<arch>/mrld
```

`mrld` is the compiled binary name of the Eamerald CLI (built from `mrld/` in this repo).

`make build` compiles for your host platform by default. To target a different platform, set `GOOS`/`GOARCH`, e.g. `GOOS=linux GOARCH=amd64 make build`. The exact output path is listed in the `building binary=...` build log line, or in `dist/artifacts.json`.

### Running with Docker

  You can run as a Docker container:

```console 
$ docker run -it --rm ghcr.io/threehook/eamerald:latest --help
```

## Quickstart

These instructions help you get Eamerald up and running as the authorizer for a sample Todo app.

### Install Eamerald authorizer container image

The Eamerald authorizer is packaged as a Docker container. You can get the latest image using the following command:

```console
$ mrld install
```

**NOTE:** If you get the following errors/warnings from Eamerald commands:

`Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?`

Be sure to allow the default Docker socket to be used in your Docker Desktop Advanced settings.

### Install the todo template

Eamerald has a set of pre-built templates that contain three types of artifacts:
* an authorization policy
* a domain model (in the form of a manifest file)
* sample data (users, groups, objects, relationships)

You can use the CLI to install the todo template:

```console
$ mrld templates install todo
```

#### Artifacts

This command will install the following artifacts in `$HOME/.config/eamerald/`:

```console
$ tree $HOME/.config/eamerald
/Users/ogazitt/.config/eamerald
├── cfg
│   └── todo.yaml
├── todo
│   ├── data
│   │   ├── citadel_objects.json
│   │   ├── citadel_relations.json
│   │   ├── todo_objects.json
│   │   └── todo_relations.json
│   └── model
│       └── manifest.yaml
└── topaz.json
```
* `cfg/todo.yaml` contains an Eamerald configuration file which references the sample Todo **policy image**. A policy image is an OCI image that contains an OPA policy. For the Todo template, this is the public GHCR image `ghcr.io/aserto-policies/policy-todo:latest`. The source code for the policy image can be found [here](https://github.com/aserto-templates/policy-todo/tree/main/content/src/policies).
* `todo/data/` contains the objects and relations for the Todo template - in this case, a set of 5 users and 4 groups that are based on the "Rick & Morty" cartoon.
* `todo/model/manifest.yaml` contains the manifest file which describes the domain model.

```console
$ tree ~/.local/share/eamerald
/Users/ogazitt/.local/share/eamerald
├── certs
│   ├── gateway-ca.crt
│   ├── gateway.crt
│   ├── gateway.key
│   ├── grpc-ca.crt
│   ├── grpc.crt
│   └── grpc.key
├── db
│   └── todo.db
└── tmpl
    └── todo
        ├── data
        │   ├── citadel_objects.json
        │   ├── citadel_relations.json
        │   ├── todo_objects.json
        │   └── todo_relations.json
        └── model
            └── manifest.yaml
```

* `certs/` contains a set of generated self-signed certificates for Eamerald.
* `db/todo.db` contains the embedded database which houses the model and data.
* `tmpl/todo` contains the template artifacts.

For a deeper overview of the `cfg/config.yaml` file, see [Eamerald configuration](docs/config.md).

#### What just happened?

Besides laying down the artifacts mentioned, installing the Todo template did the following things:

* started Eamerald in daemon (background) mode (see `mrld start --help`).
* set the manifest found in `model/manifest.yaml` (see `mrld directory set manifest --help`).
* imported the objects and relations found in `data/` (see `mrld directory import --help`).
* opened a browser window to the Eamerald [console](https://localhost:8080/ui/directory) (see `mrld console --help`).

Feel free to play around with the Eamerald console! Or follow the next few steps to interact with the Eamerald policy and authorization endpoints.

### Issue an API call

To verify that Eamerald is running with the right policy image, you can issue a `curl` call to interact with the REST API.

### Issue an authorization request

Issue an authorization request using the AuthZEN Access Evaluation API to verify that the user Rick is allowed to GET the list of todos (with `opa.policy_root: todoApp.GET.todos` configured, since that bundle has more than one policy root):

```console
$ curl -k -X POST 'https://localhost:8383/access/v1/evaluation' \
-H 'Content-Type: application/json' \
-d '{
     "subject": {"type": "user", "id": "rick@the-citadel.com"},
     "action": {"name": "allowed"},
     "resource": {"type": "todos"}
}'
```

### Run the sample application

To run the sample Todo backend in the language of your choice, and see how Eamerald is used to authorize requests, check out the [Todo template's source](https://github.com/aserto-templates/policy-todo).

To start an interactive session with the Eamerald endpoints over gRPC, see the [gRPC endpoints](#grpc-endpoints) section.

## Command line options

```console
$ mrld --help

Usage: mrld <command> [flags]

Eamerald CLI

Commands:
  start              start eamerald instance (daemon mode)
  stop               stop eamerald instance
  restart            restart eamerald instance
  status             status of eamerald daemon process
  config             configure eamerald instance
  run                start eamerald instance (console mode)
  templates          template commands
  console            open eamerald console in the browser
  directory (ds)     directory service commands
  authorizer (az)    authorizer service commands
  access (ac)        access service commands
  certs              certificate management
  install            install eamerald container
  uninstall          uninstall eamerald container
  update             update eamerald container version
  version            version information

Flags:
  -h, --help         Show context-sensitive help.
  -N, --no-check     disable local container status check ($EAMERALD_NO_CHECK)
      --no-color     disable colored terminal output ($EAMERALD_NO_COLOR)
  -v, --verbosity    log level

Run "mrld <command> --help" for more information on a command.
```

## gRPC Endpoints

To interact with the authorizer endpoint, install [grpcui](https://github.com/fullstorydev/grpcui) or [grpcurl](https://github.com/fullstorydev/grpcurl) and point them to `localhost:8282`:

```console
$ grpcui --insecure localhost:8282
```

To interact with the directory endpoint, use `localhost:9292`:

```console
$ grpcui --insecure localhost:9292
```

## Credits

Eamerald uses a lot of great and amazing open source projects and libraries.

A big thank you to all of them!

## Contribution Guidelines

Eamerald is a work in progress - if something is broken or there's a feature that you want, please file an issue and if so inclined submit a PR!

We welcome contributions from the community! Here are some general guidelines:

* File an issue first prior to submitting a PR!
* Ensure all exported items are properly commented
* If applicable, submit a test suite against your PR
