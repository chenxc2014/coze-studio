FROM debian@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc
COPY minio mc /usr/bin/
EXPOSE 9000 9001
CMD ["/usr/bin/minio", "server", "/data", "--console-address", ":9001"]
