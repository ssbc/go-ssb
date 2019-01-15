package ssb

type Authorizer interface {
	Authorize(remote *FeedRef) error
}
