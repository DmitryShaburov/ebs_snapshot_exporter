FROM golang:1.15.7-buster AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY ebs_snapshot_exporter.go ebs_snapshot_exporter.go
RUN go build .

FROM debian:buster-20210111-slim as app
RUN apt-get update \
    && apt-get -y install ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*
USER 59000:59000
COPY config.yaml /etc/ebs_snapshot_exporter/config.yaml
COPY --from=build /src/ebs_snapshot_exporter /ebs_snapshot_exporter
EXPOSE 9608
CMD ["/ebs_snapshot_exporter", "--config.file", "/etc/ebs_snapshot_exporter/config.yaml"]
