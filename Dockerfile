FROM centos:7
ADD _output/bin/aws_tags_exporter /usr/bin
ENTRYPOINT ["/usr/bin/aws_tags_exporter"]
