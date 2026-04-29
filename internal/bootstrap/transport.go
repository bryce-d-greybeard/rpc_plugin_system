package bootstrap

import "io"

type Transport interface {
	Endpoint() string
	OpenWriter() (io.WriteCloser, error)
	OpenReader() (io.ReadCloser, error)
	Cleanup() error
}
