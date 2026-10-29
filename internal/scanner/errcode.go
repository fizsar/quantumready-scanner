package scanner

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"syscall"

	"github.com/fizsar/quantumready-saas/internal/scanner/rawtls"
)

// Stable error codes (ADR-0001, schema 2). A report translates and groups by
// the code; the library's message stays beside it only as evidence, because
// it is free English text that changes between Go versions. Adding a code is
// fine; renaming one breaks stored results.
//
//	alert:<n>              the server sent TLS alert n (IANA description)
//	timeout                no answer in time
//	canceled               the scan was cancelled
//	closed                 the server closed the connection
//	reset                  the connection was reset
//	connection_refused     TCP connection refused
//	unreachable            network or host unreachable
//	connect_error          another TCP connection failure
//	blocked_address        the anti-SSRF policy refused the address
//	budget                 not sent: the raw probe budget was used up
//	unoffered_<what>       the server picked a version, cipher_suite, group,
//	                       compression or extension that was not offered
//	other_group            a group probe negotiated another group
//	other_version          a version probe got another version
//	no_hello_retry         a HelloRetryRequest was expected and did not come
//	hello_retry            a HelloRetryRequest came where the probe cannot answer it
//	no_key_share           the certificate probe has no key share for any accepted group
//	no_certificate         the server sent no certificate
//	malformed              the answer does not parse
//	not_tls                the answer is not TLS
//	read_limit             the server sent more than the read limit
//	no_common_version      crypto/tls had no version to offer
//	tls_error              another crypto/tls error
//	error                  anything else
func errorCode(err error) string {
	if err == nil {
		return ""
	}
	if code, ok := remoteAlert(err); ok {
		return fmt.Sprintf("alert:%d", code)
	}
	var ae *alertError
	if errors.As(err, &ae) {
		return fmt.Sprintf("alert:%d", ae.alert.Description)
	}
	var ue *rawtls.UnofferedError
	var oe *outcomeError
	var ce *connectError
	var nerr net.Error
	switch {
	case errors.Is(err, errBudget):
		return "budget"
	case errors.Is(err, ErrBlockedAddress):
		return "blocked_address"
	case errors.As(err, &oe):
		return oe.code
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &nerr) && nerr.Timeout():
		return "timeout"
	case errors.As(err, &ce):
		switch {
		case errors.Is(err, syscall.ECONNREFUSED):
			return "connection_refused"
		case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
			return "unreachable"
		}
		return "connect_error"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "closed"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		return "reset"
	case errors.As(err, &ue):
		return "unoffered_" + strings.ReplaceAll(strings.TrimPrefix(ue.What, "SSLv2 "), " ", "_")
	case errors.Is(err, rawtls.ErrMalformed):
		return "malformed"
	case errors.Is(err, rawtls.ErrLimit):
		return "read_limit"
	case errors.Is(err, rawtls.ErrRetry):
		return "hello_retry"
	}
	msg := err.Error()
	for _, m := range []struct{ text, code string }{
		{"server selected unsupported protocol version", "unoffered_version"},
		{"server chose an unconfigured cipher suite", "unoffered_cipher_suite"},
		{"server selected unsupported group", "unoffered_group"},
		{"server selected an unsupported group", "unoffered_group"},
		{"server selected unsupported compression", "unoffered_compression"},
		{"no supported versions", "no_common_version"},
		{"first record does not look like a TLS handshake", "not_tls"},
		{"tls: ", "tls_error"},
	} {
		if strings.Contains(msg, m.text) {
			return m.code
		}
	}
	return "error"
}

// remoteAlert is the description of an alert the server sent, as crypto/tls
// reports it: a *net.OpError{Op: "remote error"} around its unexported alert
// type, a uint8. Its level is not reported.
func remoteAlert(err error) (uint8, bool) {
	var op *net.OpError
	if !errors.As(err, &op) || op.Op != "remote error" || op.Err == nil {
		return 0, false
	}
	if v := reflect.ValueOf(op.Err); v.Kind() == reflect.Uint8 {
		return uint8(v.Uint()), true //nolint:gosec // the kind is Uint8: it fits
	}
	return 0, false
}

// alertError is a raw probe answered by an alert. Its text is as before
// schema 2 ("alert handshake_failure").
type alertError struct{ alert rawtls.Alert }

func (e *alertError) Error() string { return "alert " + e.alert.String() }

// connectError is a failed TCP connection (its text: "connect: …").
type connectError struct{ err error }

func (e *connectError) Error() string { return "connect: " + e.err.Error() }
func (e *connectError) Unwrap() error { return e.err }

// outcomeError is a handshake that worked but does not answer the probe's
// question (another group, another version), with its code.
type outcomeError struct{ code, text string }

func (e *outcomeError) Error() string { return e.text }

// trustErrorCode is the stable code of a certificate verification error.
func trustErrorCode(err error) string {
	var ua x509.UnknownAuthorityError
	var ci x509.CertificateInvalidError
	var he x509.HostnameError
	var ia x509.InsecureAlgorithmError
	var sr x509.SystemRootsError
	var uc x509.UnhandledCriticalExtension
	switch {
	case err == nil:
		return ""
	case errors.As(err, &ua):
		return "unknown_authority"
	case errors.As(err, &ci):
		switch ci.Reason {
		case x509.Expired:
			return "expired" // or not yet valid: Go gives both the same reason
		case x509.NotAuthorizedToSign:
			return "not_authorized_to_sign"
		case x509.CANotAuthorizedForThisName, x509.NameConstraintsWithoutSANs, x509.UnconstrainedName, x509.TooManyConstraints:
			return "name_constraints"
		case x509.TooManyIntermediates:
			return "too_many_intermediates"
		case x509.IncompatibleUsage, x509.CANotAuthorizedForExtKeyUsage:
			return "incompatible_usage"
		case x509.NameMismatch:
			return "name_mismatch"
		case x509.NoValidChains:
			return "no_valid_chains"
		}
		return "invalid"
	case errors.As(err, &he):
		return "hostname_mismatch"
	case errors.As(err, &ia):
		return "insecure_algorithm"
	case errors.As(err, &sr):
		return "no_roots"
	case errors.As(err, &uc):
		return "unhandled_critical_extension"
	}
	return "invalid"
}
