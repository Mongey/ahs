package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/jpillora/backoff"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// Params of the app
type Params struct {
	Backoff   *backoff.Backoff
	InputTag  string
	OutputTag string
	Separator string
}

// Clients of AWS libs
type Clients struct {
	EC2 *ec2.EC2
	MDS *ec2metadata.EC2Metadata
}

// Values computed/generated
type Values struct {
	AZ           string
	Base         string
	Hostname     string
	InstanceID   string
	Region       string
	SequentialID int
}

var start time.Time

func run(ctx *cli.Context) error {
	start = time.Now()

	logger := &Logger{
		Level:  ctx.GlobalString("log-level"),
		Format: ctx.GlobalString("log-format"),
	}

	if err := logger.Configure(); err != nil {
		panic(err)
	}

	if user, err := user.Current(); err != nil {
		return exit(cli.NewExitError("Unable to determine current user", 1))
	} else if user.Username != "root" {
		return exit(cli.NewExitError("You have to run this function as root", 1))
	}

	p := &Params{
		Backoff: &backoff.Backoff{
			Min:    100 * time.Millisecond,
			Max:    120 * time.Second,
			Factor: 2,
			Jitter: false,
		},
		InputTag:  ctx.GlobalString("input-tag"),
		OutputTag: ctx.GlobalString("output-tag"),
		Separator: ctx.GlobalString("separator"),
	}

	c := &Clients{
		EC2: nil,
		MDS: nil,
	}

	v := &Values{
		AZ:           "",
		Base:         "",
		Hostname:     "",
		InstanceID:   "",
		Region:       "",
		SequentialID: -1,
	}

	// Configure MDS Client
	if err := c.getAWSMDSClient(); err != nil {
		return exit(cli.NewExitError(err.Error(), 1))
	}

	// Fetch current AZ
	var err error
	v.AZ, err = c.getInstanceAZ()
	if err != nil {
		return exit(cli.NewExitError(err.Error(), 1))
	}

	// Compute region from AZ
	v.Region, err = computeRegionFromAZ(v.AZ)
	if err != nil {
		return exit(cli.NewExitError(err.Error(), 1))
	}

	// Configure EC2 Client
	if err := c.getAWSEC2Client(v.Region); err != nil {
		return exit(cli.NewExitError(err.Error(), 1))
	}

	// Fetch instance ID
	v.InstanceID, err = c.getInstanceID()
	if err != nil {
		return exit(cli.NewExitError(err.Error(), 1))
	}

	// Fetch the value of the input-tag and use it a base for the hostname
	for {
		v.Base, err = c.getBaseFromInputTag(p.InputTag, v.InstanceID)
		if err != nil {
			d := p.Backoff.Duration()
			if d == 60*time.Second {
				return exit(cli.NewExitError(analyzeEC2APIErrors(err), 1))
			}
			log.Infof("%s, retrying in %s", analyzeEC2APIErrors(err), d)
			time.Sleep(d)
		} else {
			p.Backoff.Reset()
			break
		}
	}

	switch ctx.Command.FullName() {
	case "instance-id":
		v.Hostname, err = computeHostnameWithInstanceID(v.Base, v.InstanceID, p.Separator, ctx.Int("length"))
	case "sequential":
		v.Hostname, v.SequentialID, err = c.computeSequentialHostname(v.Base, v.InstanceID, p.Separator, ctx.String("instance-group-tag"), ctx.String("instance-sequential-id-tag"))
	default:
		return exit(cli.NewExitError(fmt.Sprintf("Function %v is not implemented", ctx.Command.FullName()), 1))
	}

	if err != nil {
		return exit(cli.NewExitError(err.Error(), 1))
	}

	if !ctx.GlobalBool("dry-run") {
		log.Infof("Setting instance hostname locally")
		if err := setSystemHostname(v.Hostname); err != nil {
			return exit(cli.NewExitError(err.Error(), 1))
		}

		log.Infof("Setting hostname on configured instance output tag '%s'", p.OutputTag)
		if err := c.setTagValue(v.InstanceID, p.OutputTag, v.Hostname); err != nil {
			return exit(cli.NewExitError(analyzeEC2APIErrors(err), 1))
		}

		if ctx.Command.FullName() == "sequential" {
			log.Infof("Setting instance sequential id (%d) on configured tag '%s'", v.SequentialID, ctx.String("instance-sequential-id-tag"))
			if err := c.setTagValue(v.InstanceID, ctx.String("instance-sequential-id-tag"), strconv.Itoa(v.SequentialID)); err != nil {
				return exit(cli.NewExitError(analyzeEC2APIErrors(err), 1))
			}
		}
	} else {
		log.Infof("Setting instance hostname locally (dry-run)")
		log.Infof("Setting hostname on configured instance tag '%s' (dry-run)", p.OutputTag)
		if ctx.Command.FullName() == "sequential" {
			log.Infof("Setting instance sequential id (%d) on configured tag '%s' (dry-run)", v.SequentialID, ctx.String("instance-sequential-id-tag"))
		}
	}

	return exit(nil)
}

func (c *Clients) getAWSMDSClient() error {
	log.Debug("Starting AWS MDS API session")
	c.MDS = ec2metadata.New(session.New())

	if !c.MDS.Available() {
		return errors.New("Unable to access the metadata service, are you running this binary from an AWS EC2 instance?")
	}

	return nil
}

func (c *Clients) getAWSEC2Client(region string) (err error) {
	re := regexp.MustCompile("[a-z]{2}-[a-z]+-\\d")
	if !re.MatchString(region) {
		return fmt.Errorf("Cannot start AWS EC2 client session with invalid region '%s'", region)
	}

	log.Debug("Starting AWS EC2 Client session")
	c.EC2 = ec2.New(session.New(&aws.Config{
		Region: aws.String(region),
	}))
	return
}

func (c *Clients) getInstanceAZ() (az string, err error) {
	log.Debug("Fetching current AZ from MDS API")
	az, err = c.MDS.GetMetadata("placement/availability-zone")
	log.Infof("Found AZ: '%s'", az)
	return
}

func computeRegionFromAZ(az string) (region string, err error) {
	re := regexp.MustCompile("[a-z]{2}-[a-z]+-\\d[a-z]")
	if !re.MatchString(az) {
		err = fmt.Errorf("Cannot compute region from invalid availability-zone '%s'", az)
		return
	}

	region = az[:len(az)-1]
	log.Infof("Computed region : '%s'", region)
	return
}

func (c *Clients) getInstanceID() (iid string, err error) {
	log.Debug("Fetching current instance-id from MDS API")
	iid, err = c.MDS.GetMetadata("instance-id")
	log.Infof("Found instance-id : '%s'", iid)
	return
}

func (c *Clients) getBaseFromInputTag(inputTag, instanceID string) (string, error) {
	log.Infof("Querying input-tag '%s' from EC2 API", inputTag)
	instances, err := c.EC2.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []*string{
					aws.String(instanceID),
				},
			},
		},
	})

	if err != nil {
		return "", err
	}

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			for _, tag := range instance.Tags {
				if *tag.Key == inputTag {
					log.Debugf("Found input-tag '%s' : '%s' ", inputTag, *tag.Value)
					return *tag.Value, nil
				}
			}
		}
	}

	return "", fmt.Errorf("Instance doesn't contain input-tag '%s'", inputTag)
}

func analyzeEC2APIErrors(err error) string {
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return aerr.Error()
		}
		return err.Error()
	}
	return ""
}

func setSystemHostname(hostname string) error {
	return syscall.Sethostname([]byte(hostname))
}

func getSystemHostname() (string, error) {
	return os.Hostname()
}

func (c *Clients) setTagValue(instanceID, tag, value string) (err error) {
	_, err = c.EC2.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{
			aws.String(instanceID),
		},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String(tag),
				Value: aws.String(value),
			},
		},
	})

	return
}

func computeHostnameWithInstanceID(base, instanceID, separator string, length int) (string, error) {
	log.Info("Computing hostname with truncated instance-id")

	if base[len(base)-length:] == instanceID[2:2+length] {
		log.Infof("Instance ID already found in the instance tag : '%s', reusing this value", base)
		return base, nil
	}

	hostname := base + separator + instanceID[2:2+length]
	log.Infof("Computed unique hostname : '%s'", hostname)

	return hostname, nil
}

func (c *Clients) computeSequentialHostname(base, instanceID, separator, groupTag, sequentialIDTag string) (string, int, error) {
	log.Info("Computing a hostname with sequential naming")

	re := regexp.MustCompile(".*-(\\d+)$")
	if re.MatchString(base) {
		sequentialID, err := strconv.Atoi(re.FindStringSubmatch(base)[1])
		log.Infof("Current input tag value already matches '.*-\\d+$', keeping '%s' as hostname, '%d' as sequentialID", base, sequentialID)

		return base, sequentialID, err
	}

	instanceGroup, err := c.findInstanceGroupTagValue(groupTag, instanceID)
	if err != nil {
		return "", -1, err
	}

	sequentialID, err := c.findAvailableNumberInInstanceGroup(instanceGroup, groupTag, sequentialIDTag)
	if err != nil {
		return "", -1, err
	}

	hostname := base + separator + strconv.Itoa(sequentialID)
	log.Infof("Computed unique hostname : '%s' - Sequential ID : '%d'", hostname, sequentialID)

	return hostname, sequentialID, nil
}

func (c *Clients) findInstanceGroupTagValue(groupTag, instanceID string) (string, error) {
	log.Debugf("Looking up the value of the tag '%s' of the instance", groupTag)
	tags, err := c.EC2.DescribeTags(&ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("resource-type"),
				Values: []*string{
					aws.String("instance"),
				},
			},
			{
				Name: aws.String("resource-id"),
				Values: []*string{
					aws.String(instanceID),
				},
			},
			{
				Name: aws.String("key"),
				Values: []*string{
					aws.String(groupTag),
				},
			},
		},
	})

	if err != nil {
		return "", err
	}

	if len(tags.Tags) != 1 {
		return "", fmt.Errorf("Unexpected amount of tags retrieved : '%d',  expected 1", len(tags.Tags))
	}

	log.Debugf("Found instance-group value : '%s'", *tags.Tags[0].Value)
	return *tags.Tags[0].Value, nil
}

func (c *Clients) findAvailableNumberInInstanceGroup(instanceGroup, groupTag, sequentialIDTag string) (int, error) {
	log.Debugf("Looking up instances that belong to the same group")
	instances, err := c.EC2.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("tag:" + groupTag),
				Values: []*string{
					aws.String(instanceGroup),
				},
			},
		},
	})

	if err != nil {
		return -1, err
	}

	var used []int
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			for _, tag := range instance.Tags {
				if *tag.Key == sequentialIDTag {
					v, err := strconv.Atoi(*tag.Value)
					if err != nil {
						return -1, err
					}

					used = append(used, v)
					log.Debugf("Found instance '%s' with tag '%s' and sequential id '%d'", *instance.InstanceId, groupTag, v)
				}
			}
		}
	}

	sort.Ints(used)
	for i := 0; i < len(used); i++ {
		if used[i] != i+1 {
			return i + 1, nil
		}
	}

	return len(used) + 1, nil
}

func exit(err error) error {
	log.Debugf("Executed in %s, exiting..", time.Since(start))
	return err
}
