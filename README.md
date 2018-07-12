# AWS Tags Exporter

A Prometheus exporter to expose AWS tags as Prometheus metrics.

## Collectors

The table below lists all existing collectors and the information they provide.

Name    | Description | Default labels (other than tags) | Command Line Flags
--------|-------------|----------------------------------|--------------------
Autoscaling     | Exposes the tags associated with AWS Autoscaling groups in the region | autoscaling_group_name, region | autoscaling
EC2     | Exposes the tags associated with AWS EC2 instances in the region | resource_id, resource_type | ec2
EFS     | Exposes the tags associated with AWS Filesystems in the region | file_system_name, region | efs
ElasticsearchService | Exposes the tags associated with AWS Elasticsearch domains in the region | domain, region, ARN | elasticsearchservice 
ELB     | Exposes the tags associated with AWS Load Balancers in the region | load_balancer_name, region | elb
ELBv2     | Exposes the tags associated with AWS Application Load Balancers in the region | load_balancer_name, region | elbv2
RDS     | Exposes the tags associated with AWS RDS instances in the region | name, identifier, availability_zone | rds
Route53     | Exposes the tags associated with AWS Hosted Zones and DNS Healthchecks | identifier, resource_type | route53

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
