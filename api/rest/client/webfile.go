package client

import (
	"io"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

// To be submitted to cmdkit/files.

type webFile struct {
	body io.ReadCloser
	url  *url.URL
}

func newWebFile(url *url.URL) *webFile {
	return &webFile{
		url: url,
	}
}

func (wf *webFile) Read(b []byte) (int, error) {
	if wf.body == nil {
		resp, err := http.Get(wf.url.String())
		if err != nil {
			return 0, err
		}
		wf.body = resp.Body
	}
	return wf.body.Read(b)
}

func (wf *webFile) Close() error {
	if wf.body == nil {
		return nil
	}
	return wf.body.Close()
}

func (wf *webFile) FullPath() string {
	return wf.url.Host + wf.url.Path
}

func (wf *webFile) FileName() string {
	return filepath.Base(wf.url.Path)
}

func (wf *webFile) IsDirectory() bool {
	return false
}

func (wf *webFile) NextFile() (files.File, error) {
	return nil, files.ErrNotDirectory
}
