package client

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"
)

func (c *Client) do(method, path string, body io.Reader, obj interface{}) *api.Error {
	resp, err := c.doRequest(method, path, body)
	if err != nil {
		return &api.Error{Code: 0, Message: err.Error()}
	}
	return c.handleResponse(resp, obj)
}

func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.config.Timeout)
	defer cancel()

	urlpath := c.urlPrefix + "/" + strings.TrimPrefix(path, "/")
	logger.Debugf("%s: %s", method, urlpath)

	r, err := http.NewRequest(method, urlpath, body)
	if err != nil {
		return nil, err
	}
	if c.config.DisableKeepAlives {
		r.Close = true
	}

	if c.config.Username != "" {
		r.SetBasicAuth(c.config.Username, c.config.Password)
	}

	return c.client.Do(r.WithContext(ctx))
}

func (c *Client) handleResponse(resp *http.Response, obj interface{}) *api.Error {
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return &api.Error{Code: resp.StatusCode, Message: err.Error()}
	}
	logger.Debugf("Response body: %s", body)

	switch {
	case resp.StatusCode == http.StatusAccepted:
		logger.Debug("Request accepted")
	case resp.StatusCode == http.StatusNoContent:
		logger.Debug("Request suceeded. Response has no content")
	default:
		if resp.StatusCode > 399 {
			var apiErr api.Error
			err = json.Unmarshal(body, &apiErr)
			if err != nil {
				return &api.Error{
					Code:    resp.StatusCode,
					Message: err.Error(),
				}
			}
			return &apiErr
		}
		err = json.Unmarshal(body, obj)
		if err != nil {
			return &api.Error{
				Code:    resp.StatusCode,
				Message: err.Error(),
			}
		}
	}
	return nil
}
