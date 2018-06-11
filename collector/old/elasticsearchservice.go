package main


import (
  "fmt"
  "os"
  "flag"
//  "strings"
//  "log"
//  "net/http"
//  "github.com/prometheus/client_golang/prometheus"
  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/aws/session"
  "github.com/aws/aws-sdk-go/service/elasticsearchservice"
)

var (
    // Parse the args (expecting -web.listen-address -web.aws-region)
    port = flag.Int("web.listen-address", 60020, "Port number to listen on, default is 60020")
    region = flag.String("web.aws-region","","Region to get ELBs for")
)

func init() {
    flag.Parse()
}

func main() {

    fmt.Println("Listening on port",*port)
    fmt.Println("Accessing region",*region)
    // Create an EC2 service client.
    svc := elasticsearchservice.New(session.New(&aws.Config{
          Region: aws.String(*region)},
    ))

    input := &elasticsearchservice.ListDomainNamesInput {}
    result, err := svc.ListDomainNames(input)

    if err != nil {
        exitErrorf("Unable to elastic IP address, %v", err)
    }

    fmt.Println(result)
}

func exitErrorf(msg string, args ...interface{}) {
    fmt.Fprintf(os.Stderr, msg+"\n", args...)
    os.Exit(1)
}
