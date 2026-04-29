package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
)

type Transport interface {
	Endpoint() string
	OpenRequestWriter() (io.WriteCloser, error)
	OpenRequestReader() (io.ReadCloser, error)
	OpenResponseWriter() (io.WriteCloser, error)
	OpenResponseReader() (io.ReadCloser, error)
	Cleanup() error
}

func WriteRecord(w io.Writer, record Record) error {
	return json.NewEncoder(w).Encode(record)
}

func ReadRecord(r io.Reader) (Record, error) {
	var record Record
	if err := json.NewDecoder(r).Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode bootstrap record: %w", err)
	}
	return record, nil
}

func WriteResponse(w io.Writer, response Response) error {
	return json.NewEncoder(w).Encode(response)
}

func ReadResponse(r io.Reader) (Response, error) {
	var response Response
	if err := json.NewDecoder(r).Decode(&response); err != nil {
		return Response{}, fmt.Errorf("decode bootstrap response: %w", err)
	}
	return response, nil
}
