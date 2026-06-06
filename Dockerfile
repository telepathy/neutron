FROM busybox:latest
COPY bin/neutron-api-linux /neutron-api
COPY config-kind.yaml /config.yaml
EXPOSE 8888
CMD ["/neutron-api"]
