// Package irankish implements the Iran Kish IPG (REST + RSA/AES envelope,
// ikc.shaparak.ir).
//
// Credentials: [core.Config.TerminalID] is the terminal id,
// [core.Config.Username] is the acceptor id, [core.Config.Password] is the
// terminal password and [core.Config.MerchantKey] is the acquirer RSA public
// key (PEM) used to seal the authentication envelope.
package irankish

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/cryptox"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "irankish"

// Provider endpoints.
const (
	defaultBase = "https://ikc.shaparak.ir"
	tokenPath   = "/api/v3/tokenization/make"
	confirmPath = "/api/v3/confirmation/purchase"
	gatewayPath = "/iuiv3/IPG/Index/"
)

// responseCodeOK is the success code of every Iran Kish response.
const responseCodeOK = "00"

// defaultTransactionType is the operation used for a normal purchase.
const defaultTransactionType = "Purchase"

// aesKeySize is the size of the session key sealed in the envelope.
const aesKeySize = 16

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Iran Kish implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which are not part of the v3
	// tokenization API.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
	now      func() time.Time
}

var _ core.Gateway = (*Gateway)(nil)

// New builds an Iran Kish gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	switch {
	case strings.TrimSpace(cfg.TerminalID) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("TerminalID is required")
	case strings.TrimSpace(cfg.Username) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("Username (acceptor id) is required")
	case strings.TrimSpace(cfg.Password) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("Password is required")
	case strings.TrimSpace(cfg.MerchantKey) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (acquirer RSA public key) is required")
	}

	baseURL := defaultBase
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	return &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		baseURL:     baseURL,
		now:         time.Now,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Iran Kish offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodPost,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment token and returns the form to post the payer with.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	envelope, err := g.authenticationEnvelope(req.Amount.Rial())
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}

	transactionType := g.settings.transactionType
	if transactionType == "" {
		transactionType = defaultTransactionType
	}

	body := tokenRequest{
		Request: requestData{
			AcceptorID:       g.cfg.Username,
			Amount:           req.Amount.Rial(),
			PaymentID:        req.OrderID,
			RequestID:        strconv.FormatInt(g.now().UnixNano(), 10),
			RequestTimestamp: g.now().Unix(),
			RevertURI:        req.CallbackURL,
			TerminalID:       g.cfg.TerminalID,
			TransactionType:  transactionType,
		},
		AuthenticationEnvelope: envelope,
	}
	if g.settings.sendMobileAsCmsID {
		body.CmsPreservationID = req.Mobile
	}

	var out tokenResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.ResponseCode != responseCodeOK || out.Result.Token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(out.ResponseCode).WithMessage(firstNonEmpty(out.Description, Message(out.ResponseCode)))
	}

	return core.PurchaseResponse{
		Token:   out.Result.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodPost,
			URL:    transport.JoinURL(g.baseURL, gatewayPath),
			Params: map[string]string{
				"tokenIdentity": out.Result.Token,
				"merchantId":    g.cfg.Username,
			},
		},
	}, nil
}

// Verify confirms the purchase. Iran Kish keys the confirmation on the trio
// (token, retrieval reference number, system trace audit number), all three of
// which arrive with the callback.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" || req.ReferenceNumber == "" || req.TraceNumber == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token, reference number and trace number are all required")
	}

	var out confirmResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, confirmPath), confirmRequest{
		TerminalID:               g.cfg.TerminalID,
		RetrievalReferenceNumber: req.ReferenceNumber,
		SystemTraceAuditNumber:   req.TraceNumber,
		TokenIdentity:            req.Token,
	}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if out.ResponseCode != responseCodeOK || out.Result.RetrievalReferenceNumber == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(out.ResponseCode).WithMessage(firstNonEmpty(out.Description, Message(out.ResponseCode)))
	}

	amount := req.Amount
	if out.Result.Amount > 0 {
		amount = core.Rial(out.Result.Amount)
	}

	return core.VerifyResponse{
		ReferenceNumber: out.Result.RetrievalReferenceNumber,
		TransactionID:   out.Result.SystemTraceAuditNumber,
		OrderID:         req.OrderID,
		CardNumber:      firstNonEmpty(out.Result.MaskedPan, req.CardNumber),
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the fields Iran Kish posts to the revert URI.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	code := core.FirstValue(values, "responseCode", "ResponseCode")
	return core.Callback{
		Gateway:         Name,
		Succeeded:       code == responseCodeOK,
		Token:           core.FirstValue(values, "token", "tokenIdentity"),
		OrderID:         core.FirstValue(values, "paymentId", "PaymentId"),
		ReferenceNumber: core.FirstValue(values, "retrievalReferenceNumber", "RetrievalReferenceNumber"),
		TraceNumber:     core.FirstValue(values, "systemTraceAuditNumber", "SystemTraceAuditNumber"),
		CardNumber:      core.FirstValue(values, "maskedPan", "MaskedPan"),
		Code:            code,
		Message:         Message(code),
		Values:          values,
	}, nil
}

// authenticationEnvelope builds the Iran Kish envelope: the terminal, password
// and amount are encrypted with a one-time AES key, and that key plus the
// digest of the cipher text are sealed with the acquirer RSA public key.
func (g *Gateway) authenticationEnvelope(amount int64) (authenticationEnvelope, error) {
	var envelope authenticationEnvelope

	// The protocol defines the plain text as the hex string
	// terminalId + password + amount (12 digits) + "00".
	plain := fmt.Sprintf("%s%s%012d00", g.cfg.TerminalID, g.cfg.Password, amount)
	data, err := hex.DecodeString(plain)
	if err != nil {
		return envelope, core.NewError(Name, "purchase", core.ErrInvalidConfig).
			WithMessage("terminal id and password must be hexadecimal strings")
	}

	key, err := cryptox.RandomBytes(aesKeySize)
	if err != nil {
		return envelope, err
	}
	iv, err := cryptox.RandomBytes(aes.BlockSize)
	if err != nil {
		return envelope, err
	}
	cipherText, err := cryptox.AESCBCEncrypt(key, iv, data)
	if err != nil {
		return envelope, err
	}

	publicKey, err := cryptox.ParseRSAPublicKey(g.cfg.MerchantKey)
	if err != nil {
		return envelope, err
	}
	sealed, err := cryptox.EncryptPKCS1v15(publicKey, append(key, cryptox.SHA256Sum(cipherText)...))
	if err != nil {
		return envelope, err
	}

	envelope.Data = hex.EncodeToString(sealed)
	envelope.IV = hex.EncodeToString(iv)
	return envelope, nil
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Message returns the English meaning of an Iran Kish response code.
func Message(code string) string {
	messages := map[string]string{
		"00":  "approved",
		"2":   "the transaction was already reversed",
		"3":   "the acceptor is invalid",
		"5":   "the transaction was abandoned",
		"14":  "the card details are invalid",
		"25":  "the original transaction was not found",
		"30":  "the message format is invalid",
		"31":  "the payer national code does not match the card holder",
		"40":  "the requested operation is not supported",
		"42":  "the source or destination account does not accept the transaction",
		"51":  "insufficient funds",
		"54":  "the card has expired",
		"55":  "the card PIN is wrong",
		"56":  "the card was not found",
		"57":  "the terminal is not allowed to perform this transaction",
		"58":  "the terminal is not allowed to perform this transaction",
		"61":  "the amount exceeds the allowed limit",
		"62":  "the card is restricted",
		"63":  "a security rule was violated",
		"64":  "the split amounts do not add up to the transaction amount",
		"65":  "the number of transactions exceeds the allowed limit",
		"75":  "too many wrong PIN attempts",
		"77":  "the financial day of the transaction is invalid",
		"78":  "the card is not active",
		"79":  "the account linked to the card is closed or faulty",
		"86":  "the switch is signed off",
		"94":  "duplicate transaction",
		"96":  "internal switch error",
		"97":  "the CVV generation code is invalid",
		"98":  "the static second password quota is exhausted",
		"901": "the tokenization request is invalid",
		"902": "the extra request parameters are invalid",
		"903": "the payment id is invalid",
		"904": "the bill information is invalid",
		"905": "the request id is invalid",
		"906": "the request is expired",
		"907": "the revert uri is invalid",
		"909": "the acceptor is invalid",
		"910": "the split payment parameters are missing",
		"911": "the split payment parameters are invalid",
		"912": "the requested transaction is not enabled for this acceptor",
		"913": "split payments are not enabled for this acceptor",
		"914": "the caller ip address is not allowed",
		"915": "the terminal id is invalid",
		"916": "the acceptor id is invalid",
		"917": "the declared transaction type is invalid",
		"918": "the acceptor is not active",
		"919": "the split amounts violate the acceptor rules",
		"920": "the token identity is invalid",
		"921": "the token identity is invalid or expired",
		"922": "the request security was violated",
		"923": "a payment id is not allowed on a bill transaction",
		"928": "the exchanged amount is invalid",
		"929": "the payment id does not match its algorithm",
		"930": "the national code is invalid",
	}
	if message, ok := messages[code]; ok {
		return message
	}
	return "iran kish error " + code
}
