package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"
)

func (c *Client) do(method, path string, body io.Reader, obj interface{}) error {
	resp, err := c.doRequest(method, path, body)
	if err != nil {
		return &api.Error{Code: 0, Message: err.Error()}
	}
	return c.handleResponse(resp, obj)
}

func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
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

	return c.client.Do(r)
}

// eventually we may want to trigger streaming with a boolean flag in
// a single doRequest function to prevent code duplication (same for do)
func (c *Client) doStreamRequest(method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
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

	for k, v := range headers {
		r.Header.Set(k, v)
	}

	// Here are the streaming specific modifications
	// Using HTTP 2.0 to enable parallel reading req and writing resp
	r.ProtoMajor = 2
	r.ProtoMinor = 0
	r.ContentLength = -1

	return c.client.Do(r)
}

func (c *Client) doStream(method, path string, body io.Reader, headers map[string]string, obj interface{}) ([]api.AddedOutput, error) {
	resp, err := c.doStreamRequest(method, path, body, headers)
	if err != nil {
		return nil, &api.Error{Code: 0, Message: err.Error()}
	}
	return c.handleStreamResponse(resp, obj)
}

func (c *Client) handleResponse(resp *http.Response, obj interface{}) error {
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

func (c *Client) handleStreamResponse(resp *http.Response, obj interface{}) ([]api.AddedOutput, error) {
	// Read body until a termination signal (status code != 0)
	outputs := make([]api.AddedOutput, 0)
	dec := json.NewDecoder(resp.Body)
	for {
		var output api.AddedOutput
		err := dec.Decode(&output)
		outputs = append(outputs, output)
		if err != nil {
			logger.Debugf("error on decode")
			return outputs, err
		}
		if output.Code == 1 {
			return outputs, errors.New(output.Message)
		} else if output.Code == 2 {
			// All done
			logger.Debugf("add output transfer complete")
			return outputs, nil
		} else if output.Code == 0 {
			// TODO more mature handling of printing: e.g. in another function / combine with existing printers
			continue
		} else {
			return outputs, fmt.Errorf("unexpected error code: %d", output.Code)
		}
	}
}
