package main

import (
  "flag"
  "fmt"
  "os"

  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/service/elb"
  "github.com/aws/aws-sdk-go/aws/session"
)

var (
  region = flag.String("web.aws-region","","Region to get ELBs for")
)

func collectELBTags() (elbTags map[string]map[string]string, err error){
  // Create an EC2 service client.
  svc := elb.New(session.New(&aws.Config{
        Region: aws.String(*region)},
  ))

  // Get names of all load balancers
  input := &elb.DescribeLoadBalancersInput{
      LoadBalancerNames: []*string{
      },
  }
  result, err := svc.DescribeLoadBalancers(input)

  // For each ELB get their tags and store in the map
  // Create a nested map to store the tags and their values for each ELB
  var TagsNames = map[string]map[string]string{}

  for _, lbname := range result.LoadBalancerDescriptions {
      tagsInput := &elb.DescribeTagsInput{
          LoadBalancerNames: []*string{
              aws.String(*lbname.LoadBalancerName),
          },
      }
      tagsResult, err := svc.DescribeTags(tagsInput)
      if err != nil {
        exitErrorf("There is an error")
      }
      TagsNames[*lbname.LoadBalancerName] = make(map[string]string)

      for _, tag := range tagsResult.TagDescriptions {
        for _, AllTags := range tag.Tags {
          TagsNames[*lbname.LoadBalancerName][*AllTags.Key] = *AllTags.Value
        }
      }
  }
  return TagsNames, err
}

func init() {
  flag.Parse()
}

func main() {
  result, err := collectELBTags()
  if err != nil {
    exitErrorf("There is an error")
  }
  fmt.Println(result)
}

func exitErrorf(msg string, args ...interface{}) {
    fmt.Fprintf(os.Stderr, msg+"\n", args...)
    os.Exit(1)
}
