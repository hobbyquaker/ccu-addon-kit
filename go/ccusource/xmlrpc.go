// Package ccusource is the classic CCU's side of litesource.Source: the interface processes of a
// CCU3 or OpenCCU over XML-RPC (listDevices, the paramset descriptions, getParamset, and the
// events through an init callback on the loopback), and the names and rooms from ReGaHSS
// (an HM-Script over rega.exe on port 8181). An addon written against the Source interface runs
// on openccu-lite with litesource and on a classic CCU with this package.
package ccusource

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Fault is an XML-RPC fault answer.
type Fault struct {
	Code    int
	Message string
}

func (f *Fault) Error() string { return fmt.Sprintf("fault %d: %s", f.Code, f.Message) }

// --- encoding --------------------------------------------------------------------------

// marshalValue writes one value the way the CCU's daemons read it: strings, ints (i4), floats
// (double), booleans, []any (array), map[string]any (struct).
func marshalValue(b *bytes.Buffer, v any) error {
	b.WriteString("<value>")
	defer b.WriteString("</value>")
	switch x := v.(type) {
	case nil:
		b.WriteString("<string></string>")
	case string:
		b.WriteString("<string>")
		xml.EscapeText(b, []byte(x))
		b.WriteString("</string>")
	case bool:
		if x {
			b.WriteString("<boolean>1</boolean>")
		} else {
			b.WriteString("<boolean>0</boolean>")
		}
	case int:
		b.WriteString("<i4>" + strconv.Itoa(x) + "</i4>")
	case int64:
		b.WriteString("<i4>" + strconv.FormatInt(x, 10) + "</i4>")
	case float64:
		b.WriteString("<double>" + strconv.FormatFloat(x, 'f', -1, 64) + "</double>")
	case []any:
		b.WriteString("<array><data>")
		for _, e := range x {
			if err := marshalValue(b, e); err != nil {
				return err
			}
		}
		b.WriteString("</data></array>")
	case []string:
		b.WriteString("<array><data>")
		for _, e := range x {
			if err := marshalValue(b, e); err != nil {
				return err
			}
		}
		b.WriteString("</data></array>")
	case map[string]any:
		b.WriteString("<struct>")
		for k, e := range x {
			b.WriteString("<member><name>")
			xml.EscapeText(b, []byte(k))
			b.WriteString("</name>")
			if err := marshalValue(b, e); err != nil {
				return err
			}
			b.WriteString("</member>")
		}
		b.WriteString("</struct>")
	default:
		return fmt.Errorf("xmlrpc: cannot encode %T", v)
	}
	return nil
}

// MarshalCall encodes a method call.
func MarshalCall(method string, params ...any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><methodCall><methodName>`)
	xml.EscapeText(&b, []byte(method))
	b.WriteString("</methodName><params>")
	for _, p := range params {
		b.WriteString("<param>")
		if err := marshalValue(&b, p); err != nil {
			return nil, err
		}
		b.WriteString("</param>")
	}
	b.WriteString("</params></methodCall>")
	return b.Bytes(), nil
}

// MarshalResponse encodes a method response with one value.
func MarshalResponse(v any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><methodResponse><params><param>`)
	if err := marshalValue(&b, v); err != nil {
		return nil, err
	}
	b.WriteString("</param></params></methodResponse>")
	return b.Bytes(), nil
}

// MarshalFault encodes a fault response.
func MarshalFault(code int, msg string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><methodResponse><fault>`)
	_ = marshalValue(&b, map[string]any{"faultCode": code, "faultString": msg})
	b.WriteString("</fault></methodResponse>")
	return b.Bytes()
}

// --- decoding --------------------------------------------------------------------------

type xValue struct {
	XMLName  xml.Name
	Inner    []byte   `xml:",innerxml"`
	Chardata string   `xml:",chardata"`
	String   *string  `xml:"string"`
	I4       *string  `xml:"i4"`
	Int      *string  `xml:"int"`
	Double   *string  `xml:"double"`
	Boolean  *string  `xml:"boolean"`
	Base64   *string  `xml:"base64"`
	DateTime *string  `xml:"dateTime.iso8601"`
	Array    *xArray  `xml:"array"`
	Struct   *xStruct `xml:"struct"`
}

type xArray struct {
	Data []xValue `xml:"data>value"`
}

type xStruct struct {
	Members []struct {
		Name  string `xml:"name"`
		Value xValue `xml:"value"`
	} `xml:"member"`
}

// decodeValue turns a parsed value into string, int, float64, bool, []any or map[string]any.
func decodeValue(v xValue) (any, error) {
	switch {
	case v.String != nil:
		return *v.String, nil
	case v.I4 != nil:
		return strconv.Atoi(strings.TrimSpace(*v.I4))
	case v.Int != nil:
		return strconv.Atoi(strings.TrimSpace(*v.Int))
	case v.Double != nil:
		f, err := strconv.ParseFloat(strings.TrimSpace(*v.Double), 64)
		if err != nil {
			return nil, err
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, errors.New("xmlrpc: a double out of range")
		}
		return f, nil
	case v.Boolean != nil:
		s := strings.TrimSpace(*v.Boolean)
		return s == "1" || strings.EqualFold(s, "true"), nil
	case v.Base64 != nil:
		return *v.Base64, nil
	case v.DateTime != nil:
		return *v.DateTime, nil
	case v.Array != nil:
		out := make([]any, 0, len(v.Array.Data))
		for _, e := range v.Array.Data {
			d, err := decodeValue(e)
			if err != nil {
				return nil, err
			}
			out = append(out, d)
		}
		return out, nil
	case v.Struct != nil:
		out := map[string]any{}
		for _, m := range v.Struct.Members {
			d, err := decodeValue(m.Value)
			if err != nil {
				return nil, err
			}
			out[m.Name] = d
		}
		return out, nil
	default:
		// a bare <value>text</value> is a string
		return strings.TrimSpace(v.Chardata), nil
	}
}

type xResponse struct {
	XMLName xml.Name `xml:"methodResponse"`
	Params  []xValue `xml:"params>param>value"`
	Fault   *xValue  `xml:"fault>value"`
}

type xCall struct {
	XMLName    xml.Name `xml:"methodCall"`
	MethodName string   `xml:"methodName"`
	Params     []xValue `xml:"params>param>value"`
}

// charsetReader takes the ISO-8859-1 the daemons declare (their texts are ASCII, names come
// from ReGa) and UTF-8.
func charsetReader(label string, r io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "iso-8859-1", "latin1", "iso8859-1", "us-ascii", "ascii":
		return &latin1Reader{r: r}, nil
	}
	return r, nil
}

type latin1Reader struct {
	r   io.Reader
	buf []byte
}

func (l *latin1Reader) Read(p []byte) (int, error) {
	if len(l.buf) == 0 {
		tmp := make([]byte, 1024)
		n, err := l.r.Read(tmp)
		if n == 0 {
			return 0, err
		}
		var out []byte
		for _, c := range tmp[:n] {
			if c < 0x80 {
				out = append(out, c)
			} else {
				out = append(out, 0xC0|c>>6, 0x80|c&0x3F)
			}
		}
		l.buf = out
		if err != nil && err != io.EOF {
			return 0, err
		}
	}
	n := copy(p, l.buf)
	l.buf = l.buf[n:]
	return n, nil
}

// UnmarshalResponse decodes a method response's value, or its fault as an error.
func UnmarshalResponse(b []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.CharsetReader = charsetReader
	var r xResponse
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("xmlrpc: %w", err)
	}
	if r.Fault != nil {
		f, err := decodeValue(*r.Fault)
		if err != nil {
			return nil, err
		}
		m, _ := f.(map[string]any)
		code, _ := m["faultCode"].(int)
		msg, _ := m["faultString"].(string)
		return nil, &Fault{Code: code, Message: msg}
	}
	if len(r.Params) == 0 {
		return nil, nil
	}
	return decodeValue(r.Params[0])
}

// UnmarshalCall decodes a method call (the callback server's side).
func UnmarshalCall(b []byte) (string, []any, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.CharsetReader = charsetReader
	var c xCall
	if err := dec.Decode(&c); err != nil {
		return "", nil, fmt.Errorf("xmlrpc: %w", err)
	}
	params := make([]any, 0, len(c.Params))
	for _, p := range c.Params {
		v, err := decodeValue(p)
		if err != nil {
			return "", nil, err
		}
		params = append(params, v)
	}
	return c.MethodName, params, nil
}

// --- the client ------------------------------------------------------------------------

// Client calls one interface process.
type Client struct {
	URL    string
	Client *http.Client
}

// Call sends a method call and decodes the answer.
func (c *Client) Call(ctx context.Context, method string, params ...any) (any, error) {
	body, err := MarshalCall(method, params...)
	if err != nil {
		return nil, err
	}
	hc := c.Client
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("xmlrpc: %s answered %s", c.URL, res.Status)
	}
	return UnmarshalResponse(data)
}
