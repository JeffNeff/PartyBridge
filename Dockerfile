# Stage 1: Build stage
FROM golang:1.18-buster AS builder

WORKDIR /project
COPY . ./
RUN cd /project/cmd/partybridge && go build -o /project/bin/be

# Stage 2: Run stage
FROM debian:buster-slim

# Copy the compiled binary from the builder stage
COPY --from=builder /project/bin/be /app/


# Set the working directory in the container
WORKDIR /app

# Command to run the executable
ENTRYPOINT ["/app/be"]
