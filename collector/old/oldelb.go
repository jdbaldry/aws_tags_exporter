package collector


import (
  "fmt"
  "os"

  "github.com/prometheus/client_golang/prometheus"
  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/aws/session"
  "github.com/aws/aws-sdk-go/service/elb"
  "github.com/prometheus/node_exporter/collector"
)

const (
  tagsSubsystem = "elb"
)

var (
  descELBTagsName = prometheus.BuildFQName(namespace, tagsSubsystem, "tags")
  descELBTagsHelp = "AWS ELB tags converted to Prometheus labels."
  descELBTagsDefaultLabels = []string {"load_balancer_name", "region"}
)

type ELBTagsCollector struct {
  tags          *prometheus.Desc
}

// // Update implements Collector and exposes AWS Tags as a constant gauge.
// func (e *ELBTagsCollector) Update(ch chan<- prometheus.Metric) {
//   if err := e.Update(ch); err != nil {
//     return err
//   }
//   return nil
// }

func ELBTagsLabelsDesc(labelKeys []string) *prometheus.Desc {
	return prometheus.NewDesc(
		descELBTagsLabelsName,
		descELBTagsLabelsHelp,
		append(descELBTagsDefaultLabels, labelKeys...),
		nil,
	)
}

func (e *ELBTagsCollector) Collect(ch chan<- prometheus.Metric, n *DescribeLoadBalancersOutput, t *DescribeTagsOutput) {
  addGauge := func(desc *prometheus.Desc, v float64, lv ...string) {
    // We need to append the load_balancer_name from n to lv
    ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, lv)
  }
  // Get back to array of sanitized labelKey strings and array of labelValue strings
  labelKeys, labelValues := awsLabelsToPrometheusLabels()
  addGauge(ELBTagsLabelsDesc(labelKeys), 1, labelValues)
}

// Not sure yet what this function should expect as input.
// It is currently built around recieving a map with strings as keys and values.
func awsLabelsToPrometheusLabels(labels map[string]string) ([]string, []string) {
  // Implementation of santizing keys and values
  // labelKeys := make([]string, len(labels))
	// labelValues := make([]string, len(labels))
	// i := 0
	// for key, value := range labels {
	// 	labelKeys[i] = sanitizeLabelName(key)
	// 	labelValues[i] = value
	// 	i++
	// }
	// return labelKeys, labelValues

}

func newELBTags(region *string) (Collector, error) {
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

  if err != nil {
      exitErrorf("Unable to get load balancer names: , %v", err)
  }

  var tagsNames = map[string]map[string]string{}

  // For each ELB get their tags and store in the map
  for _, lbname := range result.LoadBalancerDescriptions {
      tagsInput := &elb.DescribetagsInput{
          LoadBalancerNames: []*string{
              aws.String(*lbname.LoadBalancerName),
          },
      }
      tagsResult, err := svc.DescribeTags(tagsInput)
      if err != nil {
          exitErrorf("Unable to elastic IP address, %v", err)
      }
      tagsNames[*lbname.LoadBalancerName] = make(map[string]string)
      for _, tag := range tagsResult.TagDescriptions.Tags {
        tagsNames[*lbname.LoadBalancerName][*tag.Key] = *tag.Value
      }
  }
  return &tagsNames
}

func exitErrorf(msg string, args ...interface{}) {
    fmt.Fprintf(os.Stderr, msg+"\n", args...)
    os.Exit(1)
}
