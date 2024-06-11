# syntax=docker/dockerfile:1

# Build the application from source
FROM golang:1.22 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /guelph-hydro

# Deploy the application binary into a lean image
FROM alpine

WORKDIR /

COPY --from=build-stage /guelph-hydro /guelph-hydro

EXPOSE 8080

COPY ./crontab /etc/crontabs/root

# ENTRYPOINT ["/guelph-hydro"]
CMD [ "crond", "-f" ]