// AGENTV1 FILE START: bounded IMDSv2-only identity detection; no IAM credentials.
package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("EC2 metadata unavailable")
var ErrInvalid = errors.New("EC2 metadata identity invalid")
var accountPattern = regexp.MustCompile(`^[0-9]{12}$`)
var instancePattern = regexp.MustCompile(`^i-([0-9a-f]{8}|[0-9a-f]{17})$`)
var regionPattern = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)

type EC2 struct {
	client  *http.Client
	base    string
	timeout time.Duration
}

func NewEC2(timeout time.Duration) *EC2 {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: timeout}).DialContext, ResponseHeaderTimeout: timeout, DisableKeepAlives: true}
	return &EC2{&http.Client{Transport: tr, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrInvalid }}, "http://169.254.169.254", timeout}
}
func (e *EC2) Detect(ctx context.Context) (Evidence, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	tokenReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, e.base+"/latest/api/token", nil)
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	token, err := e.read(tokenReq, 4096)
	if err != nil {
		return Evidence{}, ErrUnavailable
	}
	if len(token) == 0 || strings.ContainsAny(string(token), "\r\n\x00") {
		return Evidence{}, ErrInvalid
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.base+"/latest/dynamic/instance-identity/document", nil)
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	raw, err := e.read(req, 16384)
	if err != nil {
		return Evidence{}, ErrInvalid
	}
	var doc struct {
		Account  string `json:"accountId"`
		Region   string `json:"region"`
		AZ       string `json:"availabilityZone"`
		Instance string `json:"instanceId"`
		Type     string `json:"instanceType"`
	}
	if json.Unmarshal(raw, &doc) != nil || !accountPattern.MatchString(doc.Account) || !regionPattern.MatchString(doc.Region) || !instancePattern.MatchString(doc.Instance) || !strings.HasPrefix(doc.AZ, doc.Region) || len(doc.AZ) > 64 || len(doc.Type) > 128 {
		return Evidence{}, ErrInvalid
	}
	partition := "aws"
	if strings.HasPrefix(doc.Region, "cn-") {
		partition = "aws-cn"
	}
	if strings.HasPrefix(doc.Region, "us-gov-") {
		partition = "aws-us-gov"
	}
	// Unknown AWS partitions require an explicit detector update, never a guessed ARN.
	if strings.Contains(doc.Region, "iso") {
		return Evidence{}, ErrInvalid
	}
	return Evidence{Verified: true, Provider: "aws", Platform: "aws_ec2", Account: doc.Account, Region: doc.Region, AvailabilityZone: doc.AZ, InstanceID: doc.Instance, InstanceType: doc.Type, ResourceID: "arn:" + partition + ":ec2:" + doc.Region + ":" + doc.Account + ":instance/" + doc.Instance}, nil
}
func (e *EC2) read(req *http.Request, limit int64) ([]byte, error) {
	r, err := e.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, ErrInvalid
	}
	return b, nil
}

// AGENTV1 FILE END
