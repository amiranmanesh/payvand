// Package parsian implements the Parsian Bank IPG (SOAP, pec.shaparak.ir).
//
// Credentials: [core.Config.MerchantKey] is the LoginAccount (pin) of the
// terminal, and [core.Config.IBAN] is the settlement account used by the
// multiplexed service.
package parsian

import (
	"context"
	"encoding/binary"
	"net/http"
	"strconv"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/cryptox"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/soap"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "parsian"

// Provider endpoints and SOAP namespaces.
const (
	defaultBase = "https://pec.shaparak.ir"

	salePath        = "/NewIPGServices/Sale/SaleService.asmx"
	multiplexedPath = "/NewIPGServices/MultiplexedSale/OnlineMultiplexedSalePaymentService.asmx"
	confirmPath     = "/NewIPGServices/Confirm/ConfirmService.asmx"
	reversalPath    = "/NewIPGServices/Reverse/ReversalService.asmx"
	gatewayPath     = "/NewIPG/"

	saleNamespace        = "https://pec.Shaparak.ir/NewIPGServices/Sale/SaleService"
	multiplexedNamespace = "https://pec.Shaparak.ir/NewIPGServices/Sale/OnlineMultiplexedSalePaymentService"
	confirmNamespace     = "https://pec.Shaparak.ir/NewIPGServices/Confirm/ConfirmService"
	reversalNamespace    = "https://pec.Shaparak.ir/NewIPGServices/Reverse/ReversalService"
)

// statusOK is the success status of every Parsian operation.
const statusOK = 0

// payIDModulus bounds the generated settlement identifier to 16 digits.
const payIDModulus = 1_000_000_000_000_0000

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Parsian implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Inquiry, which Parsian does not expose.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Parsian gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (login account) is required")
	}

	baseURL := defaultBase
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	gateway := &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		baseURL:     baseURL,
	}
	if gateway.settings.splitToConfigIBAN && cfg.IBAN == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("IBAN is required when settlement to IBAN is enabled")
	}
	return gateway, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Parsian offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Multiplexing:   true,
		Currencies:     []core.Currency{core.IRR},
	}
}

// multiplexed reports whether the split settlement service must be used.
func (g *Gateway) multiplexed() bool {
	return len(g.settings.shares) > 0 || g.settings.splitToConfigIBAN
}

// Purchase creates a payment and returns the Parsian payment page URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	originator := ""
	if g.settings.sendMobileAsOriginator {
		originator = req.Mobile
	}

	var out saleResponse
	var res transport.Response
	var err error

	if g.multiplexed() {
		shares, shareErr := g.shares(req.Amount.Rial())
		if shareErr != nil {
			return core.PurchaseResponse{}, core.NewError(Name, "purchase", shareErr)
		}
		res, err = soap.Do(ctx, g.client, soap.Call{
			Endpoint:  transport.JoinURL(g.baseURL, multiplexedPath),
			Action:    multiplexedNamespace + "/MultiplexedSaleWithIBANPaymentRequest",
			Namespace: multiplexedNamespace,
			Payload: multiplexedSaleRequest{RequestData: multiplexedRequestData{
				LoginAccount:        g.cfg.MerchantKey,
				Amount:              req.Amount.Rial(),
				OrderID:             req.OrderID,
				CallBackURL:         req.CallbackURL,
				AdditionalData:      g.settings.additionalData,
				Originator:          originator,
				MultiplexedAccounts: shares,
			}},
		}, &out)
	} else {
		res, err = soap.Do(ctx, g.client, soap.Call{
			Endpoint:  transport.JoinURL(g.baseURL, salePath),
			Action:    saleNamespace + "/SalePaymentRequest",
			Namespace: saleNamespace,
			Payload: saleRequest{RequestData: saleRequestData{
				LoginAccount:   g.cfg.MerchantKey,
				Amount:         req.Amount.Rial(),
				OrderID:        req.OrderID,
				CallBackURL:    req.CallbackURL,
				AdditionalData: g.settings.additionalData,
				Originator:     originator,
			}},
		}, &out)
	}
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Result.Status != statusOK || out.Result.Token == 0 {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Result.Status)).WithMessage(out.Result.Message)
	}

	token := strconv.FormatInt(out.Result.Token, 10)
	return core.PurchaseResponse{
		Token:   token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, gatewayPath),
			Params: map[string]string{"Token": token},
		},
	}, nil
}

// shares builds the split settlement rows of a multiplexed sale.
func (g *Gateway) shares(amount int64) ([]multiplexedRow, error) {
	definitions := g.settings.shares
	if len(definitions) == 0 {
		definitions = []Share{{IBAN: g.cfg.IBAN, Amount: amount}}
	}

	rows := make([]multiplexedRow, 0, len(definitions))
	for _, share := range definitions {
		payID := share.PayID
		if payID == 0 {
			generated, err := randomPayID()
			if err != nil {
				return nil, err
			}
			payID = generated
		}
		rows = append(rows, multiplexedRow{AccountWithIBAN: accountWithIBAN{
			IBAN:   share.IBAN,
			Amount: share.Amount,
			PayID:  payID,
		}})
	}
	return rows, nil
}

// Verify confirms the payment identified by the token. Parsian rolls a
// transaction back automatically when it is not confirmed in time.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	var out confirmResponse
	res, err := soap.Do(ctx, g.client, soap.Call{
		Endpoint:  transport.JoinURL(g.baseURL, confirmPath),
		Action:    confirmNamespace + "/ConfirmPayment",
		Namespace: confirmNamespace,
		Payload: confirmRequest{RequestData: tokenNamed{
			LoginAccount: g.cfg.MerchantKey,
			Token:        req.Token,
		}},
	}, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if out.Result.Status != statusOK {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Result.Status))
	}
	if out.Result.RRN <= 0 {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrUnexpectedResponse).
			WithMessage("the confirmation carried no retrieval reference number")
	}

	return core.VerifyResponse{
		ReferenceNumber: strconv.FormatInt(out.Result.RRN, 10),
		TransactionID:   req.Token,
		OrderID:         req.OrderID,
		CardNumber:      out.Result.CardNumberMasked,
		Amount:          req.Amount,
		Raw:             res.Body,
	}, nil
}

// Refund reverses a payment that was confirmed within the reversal window.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	token := req.Token
	if token == "" {
		token = req.TransactionID
	}
	if token == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	var out reversalResponse
	res, err := soap.Do(ctx, g.client.NoRetry(), soap.Call{
		Endpoint:  transport.JoinURL(g.baseURL, reversalPath),
		Action:    reversalNamespace + "/ReversalRequest",
		Namespace: reversalNamespace,
		Payload: reversalRequest{RequestData: tokenNamed{
			LoginAccount: g.cfg.MerchantKey,
			Token:        token,
		}},
	}, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if out.Result.Status != statusOK {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Result.Status)).WithMessage(out.Result.Message)
	}

	return core.RefundResponse{RefundID: token, Amount: req.Amount, Raw: res.Body}, nil
}

// ParseCallback reads the fields Parsian posts to the callback URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "status", "Status")
	return core.Callback{
		Gateway:         Name,
		Succeeded:       status == "0",
		Token:           core.FirstValue(values, "Token", "token"),
		OrderID:         core.FirstValue(values, "OrderId", "orderId"),
		ReferenceNumber: core.FirstValue(values, "RRN", "rrn"),
		TraceNumber:     core.FirstValue(values, "TraceNumber", "STraceNo"),
		CardNumber:      core.FirstValue(values, "HashCardNumber", "CardNumberMasked"),
		Code:            status,
		Values:          values,
	}, nil
}

// randomPayID generates the 16 digit settlement identifier a multiplexed row
// needs when the caller did not pin one.
func randomPayID() (int64, error) {
	raw, err := cryptox.RandomBytes(8)
	if err != nil {
		return 0, err
	}
	value := binary.BigEndian.Uint64(raw) % payIDModulus
	if value == 0 {
		value = 1
	}
	return int64(value), nil
}
