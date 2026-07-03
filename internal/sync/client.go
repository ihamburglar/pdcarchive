package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	appToken   string
	httpClient *http.Client
}

type ColumnMeta struct {
	Name         string `json:"name"`
	FieldName    string `json:"fieldName"`
	DataTypeName string `json:"dataTypeName"`
}

func NewClient(baseURL, appToken string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		appToken: appToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (c *Client) FetchColumns(datasetID string) ([]ColumnMeta, error) {
	url := fmt.Sprintf("%s/api/views/%s/columns.json", c.baseURL, datasetID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("columns fetch %s: %s", resp.Status, string(body))
	}

	var columns []ColumnMeta
	if err := json.NewDecoder(resp.Body).Decode(&columns); err != nil {
		return nil, err
	}
	return columns, nil
}

func (c *Client) FetchPage(datasetID string, offset, limit int) ([]json.RawMessage, *http.Header, error) {
	url := fmt.Sprintf("%s/resource/%s.json?$limit=%d&$offset=%d", c.baseURL, datasetID, limit, offset)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("resource fetch %s: %s", resp.Status, string(body))
	}

	var rows []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, nil, err
	}
	h := resp.Header.Clone()
	return rows, &h, nil
}

func (c *Client) FetchDatasetName(datasetID string) (string, error) {
	url := fmt.Sprintf("%s/api/views/%s.json", c.baseURL, datasetID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return datasetID, nil
	}

	var meta struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return datasetID, nil
	}
	if meta.Name == "" {
		return datasetID, nil
	}
	return meta.Name, nil
}

func (c *Client) setHeaders(req *http.Request) {
	if c.appToken != "" {
		req.Header.Set("X-App-Token", c.appToken)
	}
	req.Header.Set("Accept", "application/json")
}

func ParseLastModified(headers *http.Header) *time.Time {
	if headers == nil {
		return nil
	}
	raw := headers.Get("X-SODA2-Truth-Last-Modified")
	if raw == "" {
		raw = headers.Get("Last-Modified")
	}
	if raw == "" {
		return nil
	}
	t, err := http.ParseTime(raw)
	if err != nil {
		return nil
	}
	return &t
}
