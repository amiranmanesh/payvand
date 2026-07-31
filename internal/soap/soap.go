// Package soap is the minimal SOAP 1.1 client used by the bank gateways that
// still expose a webservice (Parsian, Mellat). It is built on encoding/xml and
// the shared transport, so it adds no dependency.
package soap

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Call describes one SOAP invocation.
type Call struct {
	// Endpoint is the service URL to post the envelope to.
	Endpoint string
	// Action is the value of the SOAPAction header.
	Action string
	// Namespace is bound to the "ns" prefix used by the payload element.
	Namespace string
	// Payload is the operation element, marshalled inside the SOAP body. Its
	// XMLName is expected to carry the "ns:" prefix.
	Payload any
}

// envelope is the request envelope written on the wire.
type envelope struct {
	XMLName   xml.Name `xml:"soap:Envelope"`
	XMLNsSoap string   `xml:"xmlns:soap,attr"`
	XMLNsXSI  string   `xml:"xmlns:xsi,attr"`
	XMLNsXSD  string   `xml:"xmlns:xsd,attr"`
	XMLNsNS   string   `xml:"xmlns:ns,attr"`
	Body      body     `xml:"soap:Body"`
}

// body wraps the operation element.
type body struct {
	XMLName xml.Name `xml:"soap:Body"`
	Content any
}

// responseEnvelope captures the body of the response without knowing its type.
type responseEnvelope struct {
	Body struct {
		Inner []byte `xml:",innerxml"`
		Fault *struct {
			Code   string `xml:"faultcode"`
			String string `xml:"faultstring"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// Do posts the envelope and unmarshals the response body into out.
func Do(ctx context.Context, client *transport.Client, call Call, out any) (transport.Response, error) {
	payload, err := xml.Marshal(envelope{
		XMLNsSoap: "http://schemas.xmlsoap.org/soap/envelope/",
		XMLNsXSI:  "http://www.w3.org/2001/XMLSchema-instance",
		XMLNsXSD:  "http://www.w3.org/2001/XMLSchema",
		XMLNsNS:   call.Namespace,
		Body:      body{Content: call.Payload},
	})
	if err != nil {
		return transport.Response{}, fmt.Errorf("payvand: encoding soap envelope: %w", err)
	}
	payload = append([]byte(xml.Header), payload...)

	headers := map[string]string{
		"Content-Type": "text/xml; charset=utf-8",
		"Accept":       "text/xml, multipart/related",
		"SOAPAction":   call.Action,
	}

	res, err := client.Do(ctx, http.MethodPost, call.Endpoint, payload, headers)
	if err != nil {
		return res, err
	}

	var parsed responseEnvelope
	if err := xml.Unmarshal([]byte(res.Body), &parsed); err != nil {
		return res, fmt.Errorf("%w: %s", core.ErrUnexpectedResponse, strings.TrimSpace(res.Body))
	}
	if parsed.Body.Fault != nil {
		return res, fmt.Errorf("payvand: soap fault %s: %s", parsed.Body.Fault.Code, parsed.Body.Fault.String)
	}
	if out == nil {
		return res, nil
	}
	if err := xml.Unmarshal(parsed.Body.Inner, out); err != nil {
		return res, fmt.Errorf("%w: %s", core.ErrUnexpectedResponse, strings.TrimSpace(string(parsed.Body.Inner)))
	}
	return res, nil
}

// StringResult is the shape of the rpc/encoded webservices that answer with a
// single string (every Mellat operation does).
type StringResult struct {
	// Return is the raw comma separated answer of the operation.
	Return string `xml:"return"`
}
