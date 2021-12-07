FROM python:3.9.0-buster AS build

COPY . /src
WORKDIR /src
RUN python3 setup.py bdist_wheel

FROM python:3.9.0-slim-buster


COPY --from=build /src/dist/*.whl /tmp
# hadolint ignore=DL3013

# Installing an older version of pip as we wait for a real release of gql.
RUN apt-get update && apt-get install -y gcc libc-dev --no-install-recommends && pip install --no-cache-dir pip==20.1.1 && pip install --no-cache-dir /tmp/*.whl && apt-get purge -y --auto-remove gcc libc-dev && groupadd -r etos && useradd -r -m -s /bin/false -g etos etos && opentelemetry-bootstrap --action=install

ENV OTEL_EXPORTER_OTLP_ENDPOINT http://otl-collector.etos-dev.k8s.axis.com/v1/traces
ENV OTEL_RESOURCE_ATTRIBUTES service.name=ETOS API,service.version=1.2.3,deployment.environment=development
ENV OTEL_TRACES_EXPORTER otlp_proto_http
ENV OTEL_PYTHON_EXCLUDED_URLS healthcheck,healthz,selftest/ping

USER etos
EXPOSE 8080

LABEL org.opencontainers.image.source=https://github.com/eiffel-community/etos-api
LABEL org.opencontainers.image.authors=etos-maintainers@googlegroups.com
LABEL org.opencontainers.image.licenses=Apache-2.0

ENTRYPOINT ["opentelemetry-instrument", "uvicorn", "etos_api.main:APP", "--host=0.0.0.0", "--port=8080"]
