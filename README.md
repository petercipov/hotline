# hotline
SLO computation, SLA tracking, 3rd party alerting system, designed to manage thousands of 3rd party integrations

## Project Status
-  deployable NO
-  milestone 1 features (under development)

### Running the proxy

Build the program and start it with a config file (an example is in `src/hotline-proxy/testdata/proxy-example/`):

```
cd src/hotline-proxy
go build -o hotline-proxy ./cmd
./hotline-proxy validate --config hotline-proxy.yaml
./hotline-proxy run --config hotline-proxy.yaml
```

Apps then use it as their HTTP proxy, on the `listen.proxy` address of the config. Ctrl+C stops it after the calls in progress finish. Other flags: `--trust-file` (extra trusted issuers for vendor certificates), `--log-level`, `--shutdown-timeout` and `--drain-delay` (how long new calls are still taken after the stop signal, 5 seconds unless set); `./hotline-proxy run --help` lists them. Details: [docs/proxy-program.md](docs/proxy-program.md). The proxy picks up changes to its config, specs, overlays, keys and certificates within 5 seconds, or at once on `kill -HUP`: [docs/proxy-config-reload.md](docs/proxy-config-reload.md). Health checks for Kubernetes, `/livez` and `/readyz`, answer on the `listen.admin` address: [docs/proxy-health-checks.md](docs/proxy-health-checks.md).

Tests: `go test ./...` from `src/hotline-proxy` (the proxy) or `src/hotline` (the metrics) for the fast tests, `make e2e` from the repo root for the end-to-end tests against the built program.

### Configuration API (Under development)
Config API spec is deployed [here](https://bump.sh/peter-cipov/doc/hotline-api/) for visibility.
