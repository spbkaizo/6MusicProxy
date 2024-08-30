# Start from a scratch image
FROM scratch

# Copy the statically built Go binary
COPY 6MusicProxy /6MusicProxy

# Set the binary as the entry point
ENTRYPOINT ["/6MusicProxy"]

# Define environment variables (optional)
ENV PID_FILE=/var/run/6music.pid
ENV LOG_FILE=/var/log/6music.log
ENV WORK_DIR=/var/empty/
ENV UMASK=022
