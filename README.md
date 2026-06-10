# color

Tiny web server for colorful Kubernetes demos.

It serves a single HTML page whose background color comes from the pod
hostname prefix — everything before the first dash. Pods created by a
Deployment named `blue` are named `blue-<replicaset>-<pod>`, so they serve
blue pages. Deploy `blue` and `green` side by side and you have a literal
blue/green deployment demo. The page also reports which pod served the
request, which makes Service load-balancing visible:

```
🔵This is pod default/blue-796f87cc56-9dmrx on linux/amd64, serving / for 10.244.1.7:44398.
```

A modern rewrite of [jpetazzo/color](https://github.com/jpetazzo/color),
keeping the same page format and behavior.

## Run it

```bash
# Kubernetes — one deployment, imperative:
kubectl create deployment blue --image=ghcr.io/sojoudian/color
kubectl expose deployment blue --port=80 --target-port=8080

# Kubernetes — blue/green via kustomize:
kubectl apply -k k8s/overlays/blue
kubectl apply -k k8s/overlays/green

# Docker:
docker run --rm -p 8080:8080 ghcr.io/sojoudian/color

# Locally:
go run ./cmd/color
```

## Configuration

| Variable    | Default | Purpose                                            |
|-------------|---------|----------------------------------------------------|
| `PORT`      | `8080`  | Listen port (unprivileged, works as non-root)      |
| `HOSTNAME`  | —       | Overrides the pod hostname (and hence the color)   |
| `NAMESPACE` | —       | Overrides the namespace (Downward API sets this; falls back to the service account mount) |

Endpoints: `/` serves the color page for any path; `/healthz` and `/readyz`
serve liveness and readiness probes. The binary also has a `-healthcheck`
flag used as the container `HEALTHCHECK` (distroless has no shell).

## Development

```bash
make test     # go test -race with coverage
make lint     # golangci-lint
make build    # static binary in ./bin
make docker   # local container image
```

CI (GitHub Actions) lints, tests, and scans on every push and PR, and
publishes a multi-arch image (linux/amd64, linux/arm64) to
`ghcr.io/sojoudian/color` with provenance and SBOM attestations on pushes
to `master` and on `v*.*.*` tags.

## License

MIT — see [LICENSE](LICENSE).
