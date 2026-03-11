// Package ami provides an HTTP client for the Asterisk Manager Interface (AMI)
// via Asterisk's built-in HTTP server.
//
// Asterisk exposes AMI over HTTP at:
//
//	http://<host>:<port>/rawman  — raw AMI key:value responses
//	http://<host>:<port>/mxml   — XML-formatted responses  ← used by this library
//	http://<host>:<port>/manager — HTML-formatted responses (browser use)
//
// This library targets /mxml and parses responses with encoding/xml.
// Asterisk /mxml responses look like:
//
//	<ajax-response>
//	  <response type="object" id="unknown">
//	    <generic response="Success" message="Originate successfully queued"/>
//	  </response>
//	  <response type="object" id="unknown">
//	    <generic event="Hangup" channel="SIP/1001-00000001" .../>
//	  </response>
//	</ajax-response>
package ami

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Client is an Asterisk AMI HTTP client.
type Client struct {
	baseURL    string
	username   string
	secret     string
	httpClient *http.Client
}

// Config holds the configuration for the AMI client.
type Config struct {
	// Host is the Asterisk server address (e.g. "192.168.1.100")
	Host string
	// Port is the Asterisk HTTP port (default: 8088)
	Port int
	// Username is the AMI username (defined in manager.conf)
	Username string
	// Secret is the AMI password (defined in manager.conf)
	Secret string
	// Timeout for HTTP requests (default: 10s)
	Timeout time.Duration
	// TLS enables HTTPS (requires Asterisk TLS configuration)
	TLS bool
}

// ── XML document structures ───────────────────────────────────────────────────

// xmlAjaxResponse is the top-level element returned by /mxml.
type xmlAjaxResponse struct {
	XMLName   xml.Name      `xml:"ajax-response"`
	Responses []xmlResponse `xml:"response"`
}

// xmlResponse wraps a single <response> block, which contains one <generic> element.
type xmlResponse struct {
	Type    string     `xml:"type,attr"`
	ID      string     `xml:"id,attr"`
	Generic xmlGeneric `xml:"generic"`
}

// xmlGeneric holds all AMI key/value pairs as XML attributes.
// Since attribute names are dynamic, we use a custom UnmarshalXML.
type xmlGeneric struct {
	Attrs map[string]string
}

func (g *xmlGeneric) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	g.Attrs = make(map[string]string, len(start.Attr))
	for _, attr := range start.Attr {
		g.Attrs[attr.Name.Local] = attr.Value
	}
	// Consume the closing tag
	return d.Skip()
}

// ── Public response type ──────────────────────────────────────────────────────

// Response represents a parsed AMI response.
type Response struct {
	// Fields contains the key/value pairs of the primary <generic> element
	Fields map[string]string
	// Events contains the attributes of any subsequent <generic> elements
	// (e.g. status events returned alongside a response)
	Events []map[string]string
	// Raw is the unparsed XML response body
	Raw string
}

// Get returns a field value by key (case-insensitive).
func (r *Response) Get(key string) string {
	for k, v := range r.Fields {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// IsError returns true if the "response" attribute equals "Error".
func (r *Response) IsError() bool {
	return strings.EqualFold(r.Get("response"), "Error")
}

// IsSuccess returns true if the "response" attribute equals "Success".
func (r *Response) IsSuccess() bool {
	return strings.EqualFold(r.Get("response"), "Success")
}

// ── Client construction ───────────────────────────────────────────────────────

// NewClient creates a new AMI HTTP client from the given config.
func NewClient(cfg *Config) *Client {
	if cfg.Port == 0 {
		cfg.Port = 8088
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	scheme := "http"
	if cfg.TLS {
		scheme = "https"
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil
	}

	return &Client{
		baseURL:  fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port),
		username: cfg.Username,
		secret:   cfg.Secret,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
			Jar: jar,
		},
	}
}

// ── Core action method ────────────────────────────────────────────────────────

// Params is a convenience type for AMI action parameters.
type Params map[string]string

// Action sends an AMI action to the /mxml endpoint and returns the parsed Response.
// The "action", "username", and "secret" parameters are added automatically.
//
// Example:
//
//	resp, err := client.Action(ctx, "Originate", ami.Params{
//	    "Channel":  "PJSIP/1001",
//	    "Exten":    "1002",
//	    "Context":  "default",
//	    "Priority": "1",
//	})
func (c *Client) Action(ctx context.Context, action string, params Params) (*Response, error) {
	q := url.Values{}
	q.Set("action", action)

	for k, v := range params {
		q.Set(k, v)
	}

	reqURL := fmt.Sprintf("%s/mxml?%s", c.baseURL, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ami: building request: %w", err)
	}

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ami: http request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ami: unexpected HTTP status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("ami: reading response body: %w", err)
	}

	return parseXMLResponse(body)
}

// ── XML parser ────────────────────────────────────────────────────────────────

// parseXMLResponse unmarshals an Asterisk /mxml response body into a Response.
//
// The /mxml format wraps every AMI response in:
//
//	<ajax-response>
//	  <response type="object" id="unknown">
//	    <generic response="Success" message="..." />
//	  </response>
//	  <!-- additional event <response> blocks may follow -->
//	</ajax-response>
//
// The first <generic> element's attributes become Response.Fields.
// Any subsequent <generic> elements become entries in Response.Events.
func parseXMLResponse(body []byte) (*Response, error) {
	var doc xmlAjaxResponse
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("ami: parsing XML response: %w", err)
	}

	r := &Response{
		Raw:    string(body),
		Fields: make(map[string]string),
		Events: []map[string]string{},
	}

	for i, xmlResp := range doc.Responses {
		if i == 0 {
			// First block → primary response fields
			r.Fields = xmlResp.Generic.Attrs
		} else {
			// Subsequent blocks → events
			if len(xmlResp.Generic.Attrs) > 0 {
				r.Events = append(r.Events, xmlResp.Generic.Attrs)
			}
		}
	}

	return r, nil
}
