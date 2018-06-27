package collector

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elb"
)

func TestAWSTagDescriptionToPrometheusLabels(t *testing.T) {
	lbn := "elb"
	tk, tv := "name", "elb"
	tags := make([]*elb.Tag, 1)
	tags[0] = &elb.Tag{
		Key:   &tk,
		Value: &tv,
	}
	td := &elb.TagDescription{
		LoadBalancerName: &lbn,
		Tags:             tags,
	}

	lk, lv := awsTagDescriptionToPrometheusLabels(*td)

	if lk[0] != *tags[0].Key {
		t.Errorf("Label key of tag key %s should be %s, not %s", *tags[0].Key, *tags[0].Key, lk[0])
	}
	if lv[0] != *tags[0].Value {
		t.Errorf("Label value of tag value %s should be %s, not %s", *tags[0].Value, *tags[0].Value, lv[0])
	}
}
