FROM alpine:3.18

# Install required packages
RUN apk add --no-cache bash curl openssl

# Create app directory
WORKDIR /app

# Copy the test scripts
COPY test/send-*.sh ./
RUN chmod +x send-*.sh

# Default command
CMD ["./send-telemetry.sh", "all"]
