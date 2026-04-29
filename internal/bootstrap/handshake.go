package bootstrap

import (
	"fmt"
	"time"
)

func ExchangeRecordAndResponse(t Transport, record Record) (Response, error) {
	writerDone := make(chan error, 1)
	go func() {
		w, err := t.OpenRequestWriter()
		if err != nil {
			writerDone <- err
			return
		}
		defer w.Close()
		writerDone <- WriteRecord(w, record)
	}()

	readerDone := make(chan struct {
		resp Response
		err  error
	}, 1)
	go func() {
		r, err := t.OpenResponseReader()
		if err != nil {
			readerDone <- struct {
				resp Response
				err  error
			}{err: err}
			return
		}
		defer r.Close()
		resp, err := ReadResponse(r)
		readerDone <- struct {
			resp Response
			err  error
		}{resp: resp, err: err}
	}()

	if err := <-writerDone; err != nil {
		return Response{}, fmt.Errorf("write bootstrap record: %w", err)
	}
	result := <-readerDone
	if result.err != nil {
		return Response{}, fmt.Errorf("read bootstrap response: %w", result.err)
	}
	return result.resp, nil
}

func ValidateResponse(session *Session, response Response, now time.Time) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}
	if response.Version != ProtocolVersion {
		return fmt.Errorf("protocol version mismatch: got %d want %d", response.Version, ProtocolVersion)
	}
	if response.PluginID != session.PluginID {
		return fmt.Errorf("plugin id mismatch: got %q want %q", response.PluginID, session.PluginID)
	}
	if response.SessionID != session.SessionID {
		return fmt.Errorf("session id mismatch: got %q want %q", response.SessionID, session.SessionID)
	}
	if response.Token != string(session.Token) {
		return fmt.Errorf("token mismatch")
	}
	return nil
}
