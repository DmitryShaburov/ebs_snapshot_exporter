FROM alpine:3.6 as alpine
RUN apk add -U --no-cache ca-certificates
RUN cat /etc/passwd | grep nobody > passwd.nobody

FROM scratch
WORKDIR /
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=alpine /passwd.nobody /etc/passwd
COPY config.yaml /etc/ebs_snapshot_exporter/config.yaml
COPY ebs_snapshot_exporter /ebs_snapshot_exporter
USER nobody
CMD ["/ebs_snapshot_exporter"]
