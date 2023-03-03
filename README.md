# circlegen

Circlegen dynamically generates a CircleCI config file based on your Gogen project.

## Usage

For best usage, use Gogen! If you're going rogue, put the following into your `.circleci/config.yml`:

```yml
version: 2.1

setup: true

orbs:
  continuation: circleci/continuation@0.2.0

jobs:
  setup:
    docker:
      - image: cimg/go:1.18
    steps:
      - checkout
      - run: curl -fsSL https://raw.githubusercontent.com/fresh8gaming/circlegen/trunk/install.sh | bash
      - run: circlegen
      - continuation/continue:
          configuration_path: .circleci/generated-config.yml

workflows:
  setup:
    jobs:
      - setup:
          context:
            - fresh8bot
            - artifact-registry
```

## Configuration

There are a number of options available to give you some degree of control over your pipeline. All of these are managed in a `.metadata.yml` file in the root directory of the project.

### Metadata

| Parameter              | Type      | Description                                                                                                                               |
|------------------------|-----------|-------------------------------------------------------------------------------------------------------------------------------------------|
| name                   | string    | Name of the project                                                                                                                       |
| team                   | string    | Which team owns the project                                                                                                               |
| domain                 | string    | Which domain the project is associated with                                                                                               |
| staging                | boolean   | If the project requires a staging environment                                                                                             |
| whitesourceEnabled     | boolean   | If the project should be using Whitesource for vulnerability scanning                                                                     |
| kubescoreEnabled       | boolean   | If the project should be using Kubescore for analysing K8s configuration                                                                  |
| cdEnabled              | boolean   | If the project runs with Continuous Deployment (no/less approval steps)                                                                   |
| argoAppNamesProduction | string    | Comma separated list of Argo application names that are associated with the production project. Used for HA services in multiple clusters |
| argoAppNamesStaging    | string    | Comma separated list of Argo application names that are associated with the staging project. Used for HA services in multiple clusters    |
| goVersion              | string    | Manual override of the Go version, for `cimg/go` tags, and Dockerfiles                                                                    |
| alpineVersion          | string    | Manual override of alpine version in Dockerfiles                                                                                          |
| services               | []Service | List of services defined in the project                                                                                                   |


### Service

| Parameter  | Type   | Description                                        |
|------------|--------|----------------------------------------------------|
| name       | string | Name of the service                                |
| type       | string | The type of service                                |
| ciEnabled  | string | If CI should be building and deploying the service |
| dockerfile | string | Dockerfile override for the given service          |
