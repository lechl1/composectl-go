FROM alpine:3.18

# Copy the built binary from the project's build directory (included in the build context)
COPY dc /usr/local/bin/dc
RUN chmod +x /usr/local/bin/dc

COPY pw /usr/local/bin/pw
RUN chmod +x /usr/local/bin/pw

ENTRYPOINT ["/usr/local/bin/dc"]

