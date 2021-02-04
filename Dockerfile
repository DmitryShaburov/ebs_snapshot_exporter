FROM alpine:3.6 as alpine
RUN apk add -U --no-cache ca-certificates

FROM scratch
USER 59000:59000
WORKDIR /
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY config.yaml /etc/ebs_snapshot_exporter/config.yaml
COPY ebs_snapshot_exporter /ebs_snapshot_exporter
CMD ["/ebs_snapshot_exporter", "--config.file", "/etc/ebs_snapshot_exporter/config.yaml"]
