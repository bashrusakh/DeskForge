FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

ARG BUILDARCH
WORKDIR /app
RUN apk add --no-cache tzdata
COPY ./${BUILDARCH}/release /app/
VOLUME /app/data

EXPOSE 21114
CMD ["./apimain"]
