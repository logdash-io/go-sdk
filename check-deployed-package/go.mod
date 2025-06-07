module check-deployed-package

go 1.23

require github.com/logdash-io/go-sdk/logdash v0.0.0

replace github.com/logdash-io/go-sdk/logdash => /go/src/github.com/logdash-io/go-sdk/logdash/
