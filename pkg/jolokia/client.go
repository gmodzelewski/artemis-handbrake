package jolokia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL  string
	User     string
	Password string
	Origin   string
	HTTP     *http.Client
}

func New(baseURL, user, password, origin string) *Client {
	b := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	return &Client{
		BaseURL:  b,
		User:     user,
		Password: password,
		Origin:   origin,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
}

type execRequest struct {
	Type      string        `json:"type"`
	MBean     string        `json:"mbean"`
	Operation string        `json:"operation"`
	Arguments []interface{} `json:"arguments"`
}

type jolokiaResponse struct {
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

func addressMBean(brokerName, address string) string {
	return fmt.Sprintf(`org.apache.activemq.artemis:broker="%s",component=addresses,address="%s"`, brokerName, address)
}

func (c *Client) Exec(operation, brokerName, address string) error {
	body, err := json.Marshal(execRequest{
		Type:      "exec",
		MBean:     addressMBean(brokerName, address),
		Operation: operation,
		Arguments: []interface{}{},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Origin != "" {
		req.Header.Set("Origin", c.Origin)
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jolokia http %d: %s", resp.StatusCode, string(b))
	}
	var jr jolokiaResponse
	if err := json.Unmarshal(b, &jr); err != nil {
		return fmt.Errorf("decode: %w body=%s", err, string(b))
	}
	if jr.Status != 200 {
		return fmt.Errorf("jolokia status %d: %s", jr.Status, jr.Error)
	}
	return nil
}
