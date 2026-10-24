# LogDash Go SDK Demo

Use command below to test if the published SDK works:

```
LOGDASH_API_KEY=<your-api-key> LOGDASH_PERSONAL_API_KEY=<your-personal-api-key> ./check-deployed-package/run.sh
```

`LOGDASH_API_KEY` sends the logs and metrics: the ingest key of the shared "SDK smoke tests" domain (its Settings page, "Ingest key").
The check sends them as the app `go-sdk`, so each SDK shows up as its own app on that domain.
`LOGDASH_PERSONAL_API_KEY` reads them back: a personal API key (`ldp_`) with Logs read and Metrics read, restricted to that domain.
It checks the latest version through the Go module proxy; set `LOGDASH_VERSION` to a release such as `v0.1.0`, or to a commit, to check another one.
