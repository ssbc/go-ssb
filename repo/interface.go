package repo

type Interface interface {
	GetPath(...string) string
}
