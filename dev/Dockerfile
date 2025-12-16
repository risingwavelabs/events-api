FROM ubuntu:24.04
WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates && \
    groupadd -r appuser && \
    useradd -r -g appuser appuser && \
    chown -R appuser:appuser /app

ARG TARGETARCH
COPY ./bin/events-api-${TARGETARCH} /app/events-api

USER appuser

CMD ["./events-api"]
