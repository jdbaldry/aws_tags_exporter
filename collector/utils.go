package collector

import (
	"errors"
	"regexp"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

func makeConcurrentRequests(reqs []*request.Request, service string) []error {
	var wg sync.WaitGroup
	var errs = make([]error, len(reqs))
	glog.V(4).Infof("Collecting %s", service)
	wg.Add(len(reqs))
	for i := range reqs {
		go func(i int, req *request.Request) {
			defer wg.Done()
			errs[i] = req.Send()
		}(i, reqs[i])
	}
	wg.Wait()
	return errs
}

// getAccountID returns a string of the AWS Account ID.
func getAccountID() (string, error) {
	st := sts.New(session.New(&aws.Config{}))
	out, err := st.GetCallerIdentity(&sts.GetCallerIdentityInput{})

	RequestTotalMetric.With(prometheus.Labels{"service": "sts", "region": "global"}).Inc()
	if err != nil {
		RequestErrorTotalMetric.With(prometheus.Labels{"service": "sts", "region": "global"}).Inc()
		return "", err
	}

	return *out.Account, nil
}

// sanitizeLabelName modifies the provided string to ensure it is a valid Prometheus label.
// A valid label name must match the regular expression [a-zA-Z_]([a-zA-Z0-9_])*, invalid
// characters are replaced with underscores.
func sanitizeLabelName(s string) (string, error) {
	if s == "" {
		return "", errors.New("an empty string is not a valid label name")
	}
	invalidLabelCharRE := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	s = invalidLabelCharRE.ReplaceAllString(s, "_")
	return s, nil
}
