<p align="center">
  <h1 align="center">sikalabs/auth-proxy</h1>
  <p align="center">
    <a href="https://sikalabs.com"><img alt="SikaLabs" src="https://img.shields.io/badge/SIKALABS-131480?style=for-the-badge"></a>
    <a href="https://sikalabs.com"><img alt="SikaLabs" src="https://img.shields.io/badge/-sikalabs.com-gray?style=for-the-badge"></a>
  </p>
</p>

A tiny Go reverse‑proxy that performs a **pre‑request authentication** call before forwarding traffic to an upstream service.

---

## How it works

1. **Client → proxy**
  * the proxy listens on `LISTEN_ADDR` (default `:8082`).
2. **Proxy → auth service**
  * it sends the original method, URI and (optionally) the first `MAX_BODY_SIZE_MB` MiB of the body to `AUTH_ENDPOINT` using `AUTH_METHOD` (default `POST`).
  * Extra headers are added:
    * `X‑Orig‑Uri` – the full request URI (`/path?query=1`)
    * `X‑Orig‑Method` – the HTTP verb (`GET`, `POST`, …)
3. **Decision**
  * `200 OK` → the request is forwarded to `UPSTREAM_ADDR`.
  * any other status → the response is relayed unchanged to the client.
4. **Upstream → client**
  * on success the upstream’s response goes straight back to the caller; the proxy is invisible.

---

## Environment variables
| Variable                    | Default                              | Description                                                                                                   |
|-----------------------------|--------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `LISTEN_ADDR`               | `:8082`                              | Address the proxy listens on                                                                                  |
| `UPSTREAM_ADDR`             | `http://127.0.0.1:8080`              | Target service inside the pod                                                                                 |
| `AUTH_ENDPOINT`             | `http://127.0.0.1:8181/v1/signature` | Auth service endpoint                                                                                         |
| `AUTH_METHOD`               | `POST`                               | HTTP verb used for the auth call                                                                              |
| `AUTH_INCLUDE_REGEX`        | `^/public(?:/$)`                     | Only URIs matching this require auth                                                                          |  
| `MAX_BODY_SIZE_MB`          | `30`                                 | How many MiB of the body to copy to the auth request                                                          |
| `DEBUG`                     | `false`                              | Set to `true` for verbose dumps                                                                               |
| `AUTH_FORWARD_AUTH_HEADERS` | `null`                               | Comma separated list of headers to forward to the upstream from auth service response (e.g. `Client,Service`) |
