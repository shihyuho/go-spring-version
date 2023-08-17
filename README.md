# Get the latest Spring Version in Go

[![Go Report Cart](https://goreportcard.com/badge/github.com/shihyuho/go-spring-version)](https://goreportcard.com/report/github.com/shihyuho/go-spring-version)

Get the latest Spring Boot version and its associated BOM versions, e.g. Spring Cloud, written in Go.

## Usage

### Input Variables

| Name | Description |
|------|-------------|
| boot-url | URL of Spring Boot metadata (default: `https://api.spring.io/projects/spring-boot/releases`) |
| starter-url | URL of Starter metadata (default: `https://start.spring.io`) |
| insecure | `true/false`, allow insecure metadata server connections when using SSL (default: `false`) |
| boot-version | Spring Boot version, supports semver comparison, e.g. `~3.x`, and uses the current version if left blank |
| dependencies | List of dependency identifiers to include in the generated project, can separate with commas, e.g. `cloud-starter` |
| verbose | `true/false`, provides additional detailed [outputs](#outputs), often used for debugging purposes |

### Outputs

This action provides an output of the BOM versions managed by Spring, which includes the following:

- `spring-boot`
- `spring-cloud`
- `spring-cloud-azure`
- `spring-cloud-gcp`
- `spring-cloud-services`
- `spring-modulith`
- `spring-shell`
- `codecentric-spring-boot-admin`
- `hilla`
- `sentry`
- `solace-spring-boot`
- `solace-spring-cloud`
- `testcontainers`
- `vaadin`
- `wavefron`

You can refer to the `outputs` section of [action.yml](action.yml) file for more details.

### Example

```yaml
name: Auto bump Spring version

on:
  schedule:
    - cron: "0 0 * * *"

jobs:
  spring-version:
    runs-on: ubuntu-latest
    steps:
      - id: get-spring-version
        uses: shihyuho/go-spring-version@v1
        with:
          boot-version: "~3.x"
          dependencies: "cloud-starter"
    outputs:
      spring-boot: ${{ steps.get-spring-version.outputs.spring-boot }}
      spring-cloud: ${{ steps.get-spring-version.outputs.spring-cloud }}
  current-version:
    runs-on: ubuntu-latest
    steps:
      - id: get-current-version
        run: 'echo get the current spring-boot version'
    outputs:
      spring-boot: ${{ steps.get-current-version.outputs.spring-boot }}
  bump-spring-version:
    runs-on: ubuntu-latest
    needs: [spring-version, current-version]
    if: "${{ needs.spring-version.outputs.spring-boot != needs.current-version.outputs.spring-boot }}"
    steps:
      - run: 'echo bump spring-boot to ${{ needs.spring-version.outputs.spring-boot }}'
      - run: 'echo bump spring-cloud to ${{ needs.spring-version.outputs.spring-cloud }}'
```

> Refer to [Checking Version Constraints](https://github.com/Masterminds/semver#checking-version-constraints) for more details about comparing semver versions.
