# Multi-Stage Docker buld for an optimized Docker image
# Following this builds a lightweight image which could be beneficial for serverless environment (for cold starts)

# Stage One:
# Use the official Golang image to create a build artifact.
FROM golang:1.21-alpine as builder

WORKDIR /app

# Retrieve application dependencies.
COPY go.mod go.sum ./

RUN go mod download

#------- At this point, we have all of Go's toolcahin version 1.21.1 and all the dependencies installed in this image ---

# Copy only the relevant folder
COPY FunctionEntry/ ./FunctionEntry/
COPY benchmark/ ./benchmark/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -v -o myapp ./FunctionEntry/EntryPoint.go

## Stage Two:
FROM scratch

# Only copy the binary from the build stage to the final image
COPY --from=builder /app/myapp .

# Container entry point
CMD ["./myapp"]
