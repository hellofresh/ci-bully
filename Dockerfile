FROM golang:1.11-alpine

MAINTAINER Diego Siqueira <dsi@hellofresh.com>

# Install dependencies
RUN apk add --upgrade --no-cache bash git curl \
    && go get -u github.com/hellofresh/ci-bully

RUN go build ./src/github.com/hellofresh/ci-bully
RUN go install ./src/github.com/hellofresh/ci-bully

# Command SH
CMD ["/bin/sh"]