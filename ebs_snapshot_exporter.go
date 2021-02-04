package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v3"
)

const (
	namespace = "ebs_snapshot"
)

// YAML configuration
type Config struct {
	ExportedTags map[string]string `yaml:"exported_tags,omitempty"`
	Targets      map[string]Target `yaml:"targets"`
}

type Target struct {
	Filters  []Filter `yaml:"filters,omitempty"`
	AWSCreds AWSCreds `yaml:"aws_creds"`
}

type Filter struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type AWSCreds struct {
	Region    string `yaml:"region"`
	AccessKey string `yaml:"access_key,omitempty"`
	SecretKey string `yaml:"secret_key,omitempty"`
	RoleARN   string `yaml:"role_arn,omitempty"`
}

func LoadConfig(confFile string) (*Config, error) {
	yamlReader, err := os.Open(confFile)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %s", err)
	}
	defer yamlReader.Close()

	conf := &Config{}
	decoder := yaml.NewDecoder(yamlReader)
	decoder.KnownFields(true)
	if err = decoder.Decode(conf); err != nil {
		return nil, fmt.Errorf("error parsing config file: %s", err)
	}

	return conf, nil
}

// AWS API snapshots
func NewEC2Client(awsCreds *AWSCreds) (*ec2.EC2, error) {
	config := &aws.Config{
		Region: aws.String(awsCreds.Region),
	}

	if awsCreds.AccessKey != "" && awsCreds.SecretKey != "" {
		config.Credentials = credentials.NewStaticCredentials(awsCreds.AccessKey, awsCreds.SecretKey, "")
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: *config,
	})
	if err != nil {
		return nil, err
	}

	var ec2Client *ec2.EC2
	if awsCreds.RoleARN != "" {
		creds := stscreds.NewCredentials(sess, awsCreds.RoleARN)
		ec2Client = ec2.New(sess, &aws.Config{Credentials: creds})
	} else {
		ec2Client = ec2.New(sess)
	}
	return ec2Client, nil
}

func GetSnapshots(e *ec2.EC2, awsFilters []Filter) (*ec2.DescribeSnapshotsOutput, error) {
	filters := make([]*ec2.Filter, 0, len(awsFilters))
	for _, tag := range awsFilters {
		filters = append(filters, &ec2.Filter{
			Name:   aws.String(tag.Name),
			Values: []*string{aws.String(tag.Value)},
		})
	}

	params := &ec2.DescribeSnapshotsInput{}
	if len(filters) != 0 {
		params = &ec2.DescribeSnapshotsInput{Filters: filters}
	}

	resp, err := e.DescribeSnapshots(params)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Exporter
type Exporter struct {
	exportedTags       map[string]string
	target             Target
	mutex              sync.RWMutex
	ec2                *ec2.EC2
	logger             log.Logger
	up                 *prometheus.Desc
	snapshotVolumeSize *prometheus.Desc
	snapshotStartTime  *prometheus.Desc
}

func NewExporter(name string, exportedTags map[string]string, target Target, logger log.Logger) *Exporter {
	ec2, err := NewEC2Client(&target.AWSCreds)
	if err != nil {
		level.Error(logger).Log("msg", "Error initializing EC2 client", "err", err)
		os.Exit(1)
	}

	constLabels := prometheus.Labels{"target": name}
	labels := []string{"snapshot", "volume", "region", "progress", "state"}
	for k := range exportedTags {
		labels = append(labels, k)
	}

	return &Exporter{
		exportedTags:       exportedTags,
		target:             target,
		mutex:              sync.RWMutex{},
		ec2:                ec2,
		logger:             logger,
		up:                 prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "up"), "Could the AWS EC2 API be reached.", nil, constLabels),
		snapshotVolumeSize: prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "volume_size"), "Size of volume assosicated with the EBS snapshot", labels, constLabels),
		snapshotStartTime:  prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "start_time"), "Start Timestamp of EBS Snapshot", labels, constLabels),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.snapshotVolumeSize
	ch <- e.snapshotStartTime
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	snaps, err := GetSnapshots(e.ec2, e.target.Filters)
	if err != nil {
		level.Error(e.logger).Log("msg", "Error collecting metrics from EC2 API", "err", err)
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 1)

	for _, s := range snaps.Snapshots {
		exportedLabelValues := []string{*s.SnapshotId, *s.VolumeId, e.target.AWSCreds.Region, *s.Progress, *s.State}

		for _, v := range e.exportedTags {
			found := false
			for _, t := range s.Tags {
				if *t.Key == v {
					exportedLabelValues = append(exportedLabelValues, *t.Value)
					found = true
				}
			}
			if !found {
				exportedLabelValues = append(exportedLabelValues, "")
			}
		}

		ch <- prometheus.MustNewConstMetric(e.snapshotVolumeSize, prometheus.GaugeValue, float64(*s.VolumeSize), exportedLabelValues...)
		ch <- prometheus.MustNewConstMetric(e.snapshotStartTime, prometheus.GaugeValue, float64(s.StartTime.Unix()), exportedLabelValues...)
	}
}

// Entrypoint
func main() {
	var (
		configFile    = kingpin.Flag("config.file", "EBS snapshot exporter configuration file.").Default("config.yaml").String()
		configCheck   = kingpin.Flag("config.check", "If true validate the config file and then exit.").Default().Bool()
		listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9608").String()
	)

	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("ebs_snapshot_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting ebs_snapshot_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "context", version.BuildContext())

	config, err := LoadConfig(*configFile)
	if err != nil {
		level.Error(logger).Log("msg", "Error loading config", "err", err)
		os.Exit(1)
	}

	if *configCheck {
		level.Info(logger).Log("msg", "Config file is ok exiting...")
		os.Exit(0)
	}

	level.Info(logger).Log("msg", "Loaded config file")

	for key, target := range config.Targets {
		level.Info(logger).Log("msg", "Registering target", "target", key)
		exporter := NewExporter(key, config.ExportedTags, target, logger)
		prometheus.MustRegister(exporter)
	}

	prometheus.MustRegister(version.NewCollector("ebs_snapshot_exporter"))

	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>EBS Snapshot Exporter</title></head>
             <body>
             <h1>EBS Snapshot Exporter</h1>
             <p><a href='/metrics'>Metrics</a></p>
             </body>
             </html>`))
	})
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
		os.Exit(1)
	}
}
