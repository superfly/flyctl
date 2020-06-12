FROM scratch
COPY flyctl /
ENTRYPOINT ["/flyctl"]
