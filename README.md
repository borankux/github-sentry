# GitHub Webhook Service

Simple Go web service exposing `/api/webhook/github` that validates GitHub webhook signatures via [`go-github`](https://github.com/google/go-github).

## Running

1. Copy `config.example.yml` to `config.yml` and set `github_webhook_secret`.
2. (Optional) adjust `addr` to change the listening port (defaults to `:8080`).
3. Run the service:

```bash
go run .
```

Valid GitHub webhooks POST-ed to `/api/webhook/github` will respond with `hello world`.
