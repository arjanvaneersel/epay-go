package epay

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ePayURL     = "https://www.epay.bg/"
	ePayDemoURL = "https://demo.epay.bg/"
)

// PaymentRequest represents a payment request for a client
type PaymentRequest struct {
	mu sync.RWMutex
	// page, url, cin, encoded and checksum are private, because the user shouldn't change these values. The values are to be set
	// via the NewPaymentRequest function of the API to ensure a proper request is created.
	page     string
	url      string
	cin      string
	encoded  string
	checksum string

	// Currency is the currency used for this payment request
	Currency Currency // BGN, EUR or USD

	// Amount is the sum requested of the clinet
	Amount float64

	// Description is a description of what the payment is about
	Description string

	// Invoice is the invoice number
	Invoice uint64

	// ExpirationTime is the date and time the payment request will expire
	ExpirationTime time.Time

	// URLOk is the URL where the client will be redirected to afer payment
	URLOk string

	// URLOk is the URL where the client will be redirected to afer cancelling payment
	URLCancel string

	// Language is the language in which the epay interface will be shown to the user
	Language Language // en or bg
}

// encode validates all required fields and then sets the value of encoded
func (p *PaymentRequest) encode() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	str := ""

	// Check is there is a invalid client identification number, if so return an error
	if p.cin == "" {
		return fmt.Errorf("CIN is empty")
	}
	str += fmt.Sprintf("MIN=%s\n", p.cin)

	// Check if there is an invalid invoice number, if so return an error
	if p.Invoice <= 0 {
		return fmt.Errorf("Invoice is invalid")
	}
	str += fmt.Sprintf("INVOICE=%d\n", p.Invoice)

	// Check if there is an invalid amount, if so return an error
	if p.Amount < 0.01 {
		return fmt.Errorf("Amount is invalid")
	}
	str += fmt.Sprintf("AMOUNT=%.2f\n", p.Amount)

	// Check if there is an invalid expiration time, if so return an error
	if p.ExpirationTime.IsZero() {
		return fmt.Errorf("Expiration time is invalid")
	}
	str += fmt.Sprintf("EXP_TIME=%s\n", p.ExpirationTime.Format("02.01.2006 15:04:05"))

	// Currency is optional
	if p.Currency != "" {
		str += fmt.Sprintf("CURRENCY=%s\n", p.Currency)
	}

	// Language is optional
	if p.Language != "" {
		str += fmt.Sprintf("LANGUAGE=%s\n", p.Language)
	}

	// Description is optional
	if p.Description != "" {
		str += fmt.Sprintf("LANGUAGE=%s\n", p.Description)
	}

	// Encode everything
	p.encoded = base64.StdEncoding.EncodeToString([]byte(str))
	return nil
}

// CalcChecksum calculates and sets the hmac/sha1 checksum over the encoded data of the payment
func (p *PaymentRequest) CalcChecksum(secret string) error {
	// Encode data in case there isn't any encoded data yet
	if p.encoded == "" {
		if err := p.encode(); err != nil {
			return fmt.Errorf("encoding error: %v", err)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Create a checksum with hmac
	h := hmac.New(sha1.New, []byte(secret))
	h.Write([]byte(p.encoded))
	p.checksum = hex.EncodeToString(h.Sum(nil))

	return nil
}

// API provides functionality to communicate with ePay
type API struct {
	url             string
	cin             string
	secret          string
	defaultLanguage Language
}

// PaymentOption is a custom function type used for setting optional fields of PaymentRequest
type PaymentOption func(*PaymentRequest) error

// WithExpirationTime is used to override the default expiration time of a payment
func WithExpirationTime(t time.Time) PaymentOption {
	return func(p *PaymentRequest) error {
		if t.IsZero() {
			return fmt.Errorf("invalid time")
		}

		p.ExpirationTime = t
		return nil
	}
}

// Language is a custom type to ensure a valid language is provided and set on a PaymentRequest
type Language string

// String implements the Stringer interface
func (l Language) String() string {
	return string(l)
}

// LanguageFromString converts a string to it's corresponding language
func LanguageFromString(l string) (Language, error) {
	lang := strings.Trim(strings.ToLower(l), " ")
	switch lang {
	case "english"
	case "eng":
	case "en":
		return English, nil
	case "български":
	case "бг":
	case "bulgarian":
	case "bul":
	case "bg":
		return Bulgarian, nil
	default:
		return Language(""), fmt.Errorf("unsupported language")
	}
}

var (
	// English is used to change the ePay client interface to English
	English Language = "en"
	// Bulgarian is used to change the ePay client interface to Bulgarian
	Bulgarian Language = "bg"
)

// WithLanguage overrides the default language of a PaymentRequest
func WithLanguage(l Language) PaymentOption {
	return func(p *PaymentRequest) error {
		p.Language = l
		return nil
	}
}

// Currency is a custom type to ensure a valid currency is provided and set on a PaymentRequest
type Currency string

// String implements the Stringer interface
func (p Currency) String() string {
	return string(p)
}

// CurrencyFromString converts a string to it's corresponding currency
func CurrencyFromString(c string) (Currency, error) {
	curr := strings.Trim(strings.ToLower(c), " ")
	switch(curr) {
	case "euro":
	case "eur":
		return EUR, nil
	case "bgn":
		return BGN, nil
	case "usd":
		return USD, nil
	default:
		return Currency(""), fmt.Errorf("unsupported currrency")
	}
}

var (
	// EUR means Euro
	EUR Currency = "EUR"
	// BGN means Bulgarian lev
	BGN Currency = "BGN"
	// USD means United Status Dollar
	USD Currency = "USD"
)

// WithCurrency overrides the default currency of a PaymentRequest
func WithCurrency(c Currency) PaymentOption {
	return func(p *PaymentRequest) error {
		p.Currency = c
		return nil
	}
}

// PaymentPage is a custom type to ensure a valid value is set
type PaymentPage string

var (
	// Login is used for payments requests for registered ePay users
	Login PaymentPage = "paylogin"
	// Direct is used for direct payments (credit/debit cards)
	Direct PaymentPage = "credit_paydirect"
)

// WithPage overrides the default page type of a PaymentRequest
func WithPage(pg PaymentPage) PaymentOption {
	return func(p *PaymentRequest) error {
		p.page = string(pg)
		return nil
	}
}

// NewPaymentRequest creates and prepares a new payment request
// Mandatory fields are provided as static arguments, optional fields as options
// By default the currency is EUR, expiration time is 7 days, language is English
func (api *API) NewPaymentRequest(amount float64, description string, invoice uint64, options ...PaymentOption) (*PaymentRequest, error) {
	// Create a new payment request
	p := PaymentRequest{
		page:           "credit_paydirect",
		cin:            api.cin,
		url:            api.url,
		ExpirationTime: time.Now().AddDate(0, 0, 7),
		Language:       English,
		Currency:       EUR,
		Amount:         amount,
		Description:    description,
		Invoice:        invoice,
	}

	// Loop over the options
	for _, option := range options {
		if err := option(&p); err != nil {
			return nil, err
		}
	}

	return &p, nil
}

// URL gets the url for execution of the payment request
// This function is mainly meant to be used in a template
func (p *PaymentRequest) URL() string {
	return p.url
}

// Page gets the page type for the payment request
// This function is mainly meant to be used in a template
func (p *PaymentRequest) Page() string {
	return p.page
}

// CIN gets the Client Indentification Number
// This function is mainly meant to be used in a template
func (p *PaymentRequest) CIN() string {
	return p.cin
}

// Encoded gets the encoded payment request
// This function is mainly meant to be used in a template
func (p *PaymentRequest) Encoded() string {
	return p.encoded
}

// Checksum gets the checksum of the payment request
// This function is mainly meant to be used in a template
func (p *PaymentRequest) Checksum() string {
	return p.checksum
}

// PaymentRequestHandler is a HandlerFunc for processing payment requests
// Expects to get the following data are POST or GET arguments:
// amount: The sum requested from the client (mandatory)
// description: A description what the payment is for (mandatory)
// invoice: The invoice number (mandatory)
// language: The language of epay's user interface (optional) [en*, bg]
// currency: The currency (optional) [eur*, bgn, usd]
// type: The type of payment (optional) [direct*, login]
func (api *API) PaymentRequestHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	// Get the mandatory amount
	amount, err := strconv.ParseFloat(r.FormValue("amount"), 64)
	if err != nil {
		http.Error(w, "amount is invalid or missing", http.StatusInternalServerError)
		return
	}

	// Get the mandatory description
	description := r.FormValue("description")
	if description == "" {
		http.Error(w, "description is empty", http.StatusInternalServerError)
		return
	}

	// Get the mandatory invoice number
	invoice, err := strconv.ParseUint(r.FormValue("invoice"), 10, 64)
	if err != nil {
		http.Error(w, "invoice is invalid or missing", http.StatusInternalServerError)
		return
	}

	// Create an empty slice of payment options to collect the options to be executed based upon the optional parameters
	options := []PaymentOption{}

	// Get the optional language
	l := r.FormValue("language")
	if l == "" {
		l = "en"
	}
	
	lang, err := LanguageFromString(l)
	if err != nil {
		http.Error(w, "invalid language", http.StatusInternalServerError)
		return
	}
	append(options, WithLanguage(lang))


	// Get the optional currency
	c := r.FormValue("currency")
	if c == "" {
		c = "eur"
	}

	curr, err := CurrencyFromString(c)
	if err != nil {
		http.Error(w, "invalid currency", http.StatusInternalServerError)
		return
	}
	options = append(options, WithCurrency(curr))

	// Get the optional type
	switch strings.ToLower(r.FormValue("type")) {
	// Default type is direct payment (credit / debit card)
	case "":
	case "direct":
		options = append(options, WithPage(Direct))
	// Payment Request for registered ePay users
	case "request":
		options = append(options, WithPage(Login))
	// Invalid type
	default:
		http.Error(w, "invalid type", http.StatusInternalServerError)
		return
	}

	// Create a new payment request
	data, err := api.NewPaymentRequest(amount, description, invoice, options...)

	// Calculate the checksum
	data.CalcChecksum(api.secret)

	// Open the template for payment processing
	tpl, err := template.ParseFiles("templates/simplepaymentrequest.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Execute the template
	if err := tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// PaymentStatus is a custom type to ensure a proper status
type PaymentStatus string

// String implements the Stringer interface
func (p PaymentStatus) String() string {
	return string(p)
}

var (
	// Paid means a completed payment
	Paid PaymentStatus = "PAID"

	// Denied means a failed or cancelled payment
	Denied PaymentStatus = "DENIED"

	// Expired means an expired payment
	Expired PaymentStatus = "EXPIRED"
)

// Payment is a payment done upon a payment request
type Payment struct {
	// Invoice number
	Invoice uint64

	// Status of the payment
	Status PaymentStatus

	// Payment date and time
	PayDate time.Time

	// Transaction number
	Stan int64

	// Authorization code
	Bcode string
}

// ErrInvalidInvoice is to be returned by payment handlers in case the invoice provided is invalid
var ErrInvalidInvoice = errors.New("invalid invoice")

// PaymentHandlerFunc is a custom type which represents the signature of a payment handler
// A PaymentHandlerFunc is called by PaymentCallbackHandler after it has processed the callback data received from ePay without any errors. Within a PaymentHandlerFunc
// should be the logic which connects a payment to the system the libary is being used in, e.g. storing a payment into a database.
// A successful call and general error handling is done in the normal way by either returning nil or an error, only in case that the reference provided as
// Payment.Invoice is invalid a PaymentHandlerFunc is expected to return ErrInvalidInvoice.
// This is important to guarantee that a proper answer is returned to ePay.
type PaymentHandlerFunc func(p Payment) error

// PaymentCallbackHandler returns the HandlerFunc which should be connected to the route serving the URL provided at epay as the notification URL
// It takes a PaymentHandlerFunc as an argument
func (api *API) PaymentCallbackHandler(f PaymentHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Ensure that we only accept POST calls
		if r.Method != http.MethodPost {
			http.Error(w, "invalid method", http.StatusBadRequest)
			return
		}

		// Parse the form
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get encoded and checksum via the form or parameters
		encoded := r.FormValue("encoded")
		checksum := r.FormValue("checksum")

		// Calculate the expected checksum
		h := hmac.New(sha1.New, []byte(api.secret))
		h.Write([]byte(encoded))
		expected := hex.EncodeToString(h.Sum(nil))

		// Check if the checksum is what we expected
		if checksum != expected {
			http.Error(w, fmt.Sprintf("invalid checksum %q", checksum), http.StatusBadRequest)
			log.Printf("expected checksum %q, but got %q", expected, checksum)
			return
		}

		// Decode the payload
		d, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			http.Error(w, "decoding error: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Convert the payload to a string
		data := string(d)

		status := ""

		// Split the payload on newline
		parts := strings.Split(data, "\n")

		// Create an empty payment and loop over all parts to process them
		payment := Payment{}
		for _, part := range parts {
			// Split the part by the equal sign
			e := strings.Split(part, "=")

			// The first element reprents the field name, which can be INVOICE, STATUS, PAY_TIME, STAN, BCODE
			switch e[0] {
			case "INVOICE": // Invoice number
				i, err := strconv.ParseUint(e[1], 10, 64)
				if err != nil {
					log.Printf("failed to parse invoice %v: %v", e[1], err)
					status = "ERR"
				}
				payment.Invoice = i
			case "STATUS": // Status can be PAID, DENIED or EXPIRED
				payment.Status = PaymentStatus(e[1])
			case "PAY_TIME": // Data and time of payment
				t, err := time.Parse("02.01.2006 15:04:05", e[1])
				if err != nil {
					log.Printf("failed to arse dateTime %q: %v", e[1], err)
					status = "ERR"
				}
				payment.PayDate = t
			case "STAN": // Transaction number
				s, err := strconv.ParseInt(e[1], 10, 64)
				if err != nil {
					log.Printf("failed to parse stan %v: %v", e[1], err)
					status = "ERR"
				}
				payment.Stan = s
			case "BCODE": // Authorization number
				payment.Bcode = e[1]
			}
		}

		// If there hasn't been an error PaymentHandlerFunc processing can start
		if status != "ERR" {
			// Call the PaymentHandlerFunc
			if err := f(payment); err != nil {
				// The invoice number is unkown or invalid, so status has to be set to "NO"
				if err == ErrInvalidInvoice {
					status = "NO"
				} else { // Another error occured, so the status has to be set to "ERR"
					log.Printf("payment handler error: %v", err)
					status = "ERR"
				}
			} else { // No error was returned by the PaymentHandlerFunc, so the status should be "OK"
				status = "OK"
			}
		}

		// Prepare and send the answer to the ePay server
		answer := fmt.Sprintf("INVOICE=%d:STATUS=%s\n", payment.Invoice, status)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(answer))
	}
}

// Option is an API opion
type Option func(*API) error

// WithDemoURL will set the API to use the demo instead of production URL
// It's recommended to use this option during development
func WithDemoURL() Option {
	return func(api *API) error {
		api.url = ePayDemoURL
		return nil
	}
}

// New initiates and returns an instance of the API
// Takes the Client Indentification Number (cin) and the secret key as mandatory arguments
func New(cin, secret string, options ...Option) (*API, error) {
	// Create a new API instance
	api := API{
		cin:    cin,
		secret: secret,
		url:    ePayURL,
	}

	// Loop over the provided options
	for _, option := range options {
		if err := option(&api); err != nil {
			return nil, fmt.Errorf("option error: %v", err)
		}
	}

	return &api, nil
}
