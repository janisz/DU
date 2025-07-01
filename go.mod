module github.com/janisz/DU

go 1.23.0

replace github.com/dghubble/go-twitter => github.com/janisz/go-twitter v0.0.0-20201206102041-3fe237ed29f3

require (
	github.com/avast/retry-go v3.0.0+incompatible
	github.com/dghubble/go-twitter v0.0.0-00010101000000-000000000000
	github.com/dghubble/oauth1 v0.7.3
	github.com/g8rswimmer/go-twitter/v2 v2.1.5
	github.com/gen2brain/go-fitz v1.23.7
	github.com/sirupsen/logrus v1.9.3
	golang.org/x/net v0.41.0
)

require (
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/dghubble/sling v1.3.0 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
)
