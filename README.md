# AWS Tags Exporter

A Prometheus exporter to expose AWS tags as Prometheus metrics.

## Collectors

The table below lists all existing collectors and the information they provide.

Name    | Description | Default labels (other than tags)
--------|-------------|----------------------------------
ELB     | Exposes the tags associated with Elastic Load Balancers in the region | load_balancer_name, region
RDS     | Exposes the tags associated with all AWS RDS instances in the region | name, identifier, availability_zone
elastic_service | The tags related with Elasticsearch services are exposed | domain, region

## Building and running

You can download the latest releases from the releases pane or build it yourself.

Prerequisites:

* [Go compiler](https://golang.org/dl/)
* RHEL/CentOS: `glibc-static` package.

Building:

    go get github.com/grapeshot/aws_tags_exporter
    cd ${GOPATH-$HOME/go}/src/github.com/grapeshot/aws_tags_exporter
    make
    ./aws_tags_exporter <flags>


To see all available command line flags:
    ./aws_tags_exporter -h

## Running tests

    make test

# Dependency Management
Dependencies are managed using [dep](https://github.com/golang/dep)
`make update-dependencies`

https://golang.github.io/dep/docs/daily-dep.html
