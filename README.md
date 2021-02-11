# EBS Snapshots Exporter

Export AWS EBS Snapshot metrics in Prometheus format.

## Credits

- [mr-karan/ebs-snapshot-exporter](https://github.com/mr-karan/ebs-snapshot-exporter) - for main idea, readme and parts of code
- [prometheus](https://github.com/prometheus/) - for examples how to write Prometheus exporters
- [prometheus-community/helm-charts](https://github.com/prometheus-community/helm-charts) - for examples of Helm charts for Prometheus exporters

## Features

- Ability to add ad-hoc labels in the form of AWS Tags to the exported metrics.
- Filter EBS Snapshots using standard AWS Filters.
- Ability to register multiple exporter in form of Targets to query multiple regions and AWS Accounts.
- Support for `Assume Role` while authenticating to AWS using Role ARN.

## Table of Contents

- [Getting Started](#getting-started)
  - [How it Works](#how-it-works)
  - [Installation](#installation)

- [Advanced Section](#advanced-section)
  - [Exported metrics](#exported-metrics)
  - [Flags](#flags)
  - [Configuration options](#configuation-options)
  - [Helm chart values](#helm-chart-values)
  - [Setting up Prometheus](#setting-up-prometheus)

## Getting Started

### How it Works

`ebs_snapshot_exporter` uses [AWS SDK](https://github.com/aws/aws-sdk-go) to authenticate with AWS API
and fetch Snapshots metdata. You can specify multiple `targets` to fetch EBS Snapshots data and this exporter will collect all metrics and export in the form of Prometheus metrics.

You will need an _IAM User/Role_ with the following policy attached to the server from where you are running this program:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeSnapshotAttribute",
                "ec2:DescribeSnapshots",
                "ec2:DescribeImportSnapshotTasks"
            ],
            "Resource": "*"
        }
    ]
}
```

### Installation

There are multiple ways of installing `ebs_snapshot_exporter`.

### Running from Helm chart

```bash
helm repo add ebs-snapshot-exporter https://dmitryshaburov.github.io/ebs_snapshot_exporter/
helm repo update
helm install [RELEASE_NAME] ebs-snapshot-exporter/prometheus-ebs-snapshot-exporter
```

### Running as Docker container

[dshaburov/ebs-snapshot-exporter](https://hub.docker.com/r/dshaburov/ebs-snapshot-exporter)

```bash
docker run -p 9608:9608 -v /etc/ebs_snapshot_exporter/config.yaml:/etc/ebs_snapshot_exporter/config.yaml dshaburov/ebs-snapshot-exporter:latest
```

### Precompiled binaries

Precompiled binaries for released versions are available in the [_Releases_ section](https://github.com/DmitryShaburov/ebs_snapshot_exporter/releases/).

### Compiling the binary

You can checkout the source code and build manually:

```bash
git clone https://github.com/DmitryShaburov/ebs_snapshot_exporter.git
cd ebs_snapshot_exporter
go build .
./ebs_snapshot_exporter --config.file config.yaml
```

## Advanced Section

### Exported metrics

| Metric                   | Meaning                                          | Labels                                                                  |
| ------------------------ | ------------------------------------------------ | ----------------------------------------------------------------------- |
| ebs_snapshot_up          | Could the AWS EC2 API be reached                 | target                                                                  |
| ebs_snapshot_volume_size | Size of volume assosicated with the EBS snapshot | target, region, volume, snapshot, progress, state, +_user defined tags_ |
| ebs_snapshot_start_time  | Start Timestamp of EBS Snapshot                  | target, region, volume, snapshot, progress, state, +_user defined tags_ |

### Flags

```bash
./ebs_snapshot_exporter --help
```

- __`config.file`:__ EBS snapshot exporter configuration file.
- __`config.check`:__ If true validate the config file and then exit.
- __`web.listen-address`:__ The address to listen on for HTTP requests.
- __`log.level`:__ Only log messages with the given severity or above. One of: [debug, info, warn, error]
- __`log.format`:__ Output format of log messages. One of: [logfmt, json]
- __`version`:__ Show application version.

### Configuration options

See [config.yaml](https://github.com/DmitryShaburov/ebs_snapshot_exporter/blob/main/config.yaml)
for detailed description of configuration file.

### Helm chart values

See [values.yaml](https://github.com/DmitryShaburov/ebs_snapshot_exporter/blob/main/charts/prometheus-ebs-snapshot-exporter/values.yaml)
for full list of available Helm chart values and their default configuration.

### Setting up Prometheus

You can add the following config under `scrape_configs` in Prometheus' configuration.

```yaml
  - job_name: 'ebs-snapshots'
    metrics_path: '/metrics'
    static_configs:
    - targets: ['localhost:9608']
      labels:
        service: ebs-snapshots
```

### Example Queries

- Count of snapshots: `count(ebs_snapshot_start_time{volume="<volume ID>"})`
- Last successful snapshot age: `time() - max(ebs_snapshot_start_time{state="completed"}) by (volume, target)`
- Last unsuccesful snapshot age: `time() - max(ebs_snapshot_start_time{state!="completed"}) by (volume, target)`
- Volume size of EBS for which snapshot is taken: `max(ebs_snapshot_volume_size{state="completed"}) by (volume, target)`

### Example Alerts

Alert when exporter cannot reach AWS API:

```yaml
- alert: SnapshotTargetFailed
  expr: ebs_snapshot_up == 0
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: EBS Snapshot target failed
```

Alert when last snapshot was taken more than 2 days ago

```yaml
- alert: SnapshotAge
  expr: time() - max(ebs_snapshot_start_time{state="completed",target="elasticsearch"}) by (name) > 86400 * 2
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: EBS Snapshots older than 2 days
```

## Contribution

PRs on Feature Requests, Bug fixes are welcome. Feel free to open an issue and have a discussion first. Contributions on more alert scenarios, more metrics are also welcome and encouraged.

## License

[MIT](license)
