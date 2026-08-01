// Package nodevalidator implements pre-flight validation of a proxy node
// against a specific core kind, BEFORE config generation and process start.
//
// Inspired by v2rayN's NodeValidator: instead of letting the core fail at
// startup with a cryptic error, we catch incompatible protocol + transport
// combinations early and return actionable error messages.
//
// Validation chain:
//  1. Protocol support check (core doesn't handle this protocol at all)
//  2. Transport support check (core doesn't handle this transport type)
//  3. Protocol-specific transport restriction (e.g. sing-box SS only raw/ws)
//  4. Version-specific field compatibility (delegated to compat package)
package nodevalidator

import (
	"fmt"

	"boxpanel/internal/coreinfo"
	"boxpanel/internal/models"
)

// ValidationError describes a single validation failure.
type ValidationError struct {
	Code    string `json:"code"`    // machine-readable: PROTO_UNSUPPORTED, TRANSPORT_UNSUPPORTED, SS_TRANSPORT_RESTRICTED
	Message string `json:"message"` // human-readable
	Action  string `json:"action"`  // suggested fix
}

// ValidateResult is the result of validating a server against a core kind.
type ValidateResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
}

// Validate checks whether a given server can be handled by the specified core kind.
// Returns a ValidateResult with any errors (blocking) or warnings (non-blocking).
func Validate(srv models.Server, coreKind string) ValidateResult {
	result := ValidateResult{Valid: true}
	info := coreinfo.GetInfo(coreKind)

	// 1. Protocol support check
	if !coreinfo.SupportsProtocol(coreKind, srv.Protocol) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Code:    "PROTO_UNSUPPORTED",
			Message: fmt.Sprintf("%s does not support protocol %q", info.Name, srv.Protocol),
			Action:  suggestAltCore(srv.Protocol, coreKind),
		})
		return result // no point checking further
	}

	// 2. Transport support check
	transport := srv.TransportType
	if transport != "" && transport != "tcp" && transport != "raw" {
		if !coreinfo.SupportsTransport(coreKind, transport) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "TRANSPORT_UNSUPPORTED",
				Message: fmt.Sprintf("%s does not support transport type %q", info.Name, transport),
				Action:  suggestAltCoreForTransport(srv.Protocol, transport, coreKind),
			})
		}
	}

	// 3. Protocol-specific transport restrictions
	// sing-box Shadowsocks only supports raw (no transport) and ws
	if coreKind == models.CoreKindSingBox && srv.Protocol == models.ProtoShadowsocks {
		if !coreinfo.SSCompatible(srv.TransportType) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "SS_TRANSPORT_RESTRICTED",
				Message: fmt.Sprintf("sing-box Shadowsocks only supports raw/ws transport, got %q", srv.TransportType),
				Action:  "Use Xray or mihomo for Shadowsocks with " + srv.TransportType + " transport",
			})
		}
	}

	// 4. Hysteria2 core: only hysteria2 protocol
	if coreKind == models.CoreKindHysteria2 && srv.Protocol != models.ProtoHysteria2 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Code:    "PROTO_UNSUPPORTED",
			Message: "Hysteria2 core only supports the hysteria2 protocol",
			Action:  "Switch to sing-box or mihomo for " + srv.Protocol,
		})
	}

	// 5. Warnings for known edge cases
	if coreKind == models.CoreKindSingBox && srv.TransportType == "xhttp" {
		result.Warnings = append(result.Warnings, ValidationError{
			Code:    "XHTTP_VERSION_SENSITIVE",
			Message: "xhttp transport name changed to splithttp in sing-box 1.11+; version adapter will handle this",
			Action:  "Ensure sing-box version >= 1.11.0 for splithttp support",
		})
	}

	// VMess alterId > 0 is deprecated everywhere
	if srv.Protocol == models.ProtoVMess && srv.AlterID > 0 {
		result.Warnings = append(result.Warnings, ValidationError{
			Code:    "VMESS_AID_DEPRECATED",
			Message: "VMess alterId > 0 is deprecated and removed in modern cores",
			Action:  "Set alterId to 0 for compatibility",
		})
	}

	return result
}

// ValidateOrError is a convenience wrapper that returns an error if validation fails.
func ValidateOrError(srv models.Server, coreKind string) error {
	result := Validate(srv, coreKind)
	if len(result.Errors) > 0 {
		return fmt.Errorf("node validation failed: %s", result.Errors[0].Message)
	}
	return nil
}

// suggestAltCore suggests an alternative core that supports the given protocol.
func suggestAltCore(proto, excludeKind string) string {
	for _, kind := range []string{models.CoreKindSingBox, models.CoreKindMihomo, models.CoreKindXray, models.CoreKindHysteria2} {
		if kind == excludeKind {
			continue
		}
		if coreinfo.SupportsProtocol(kind, proto) {
			return fmt.Sprintf("Switch to %s", coreinfo.GetInfo(kind).Name)
		}
	}
	return "No compatible core found for " + proto
}

// suggestAltCoreForTransport suggests an alternative core that supports both
// the given protocol and transport type.
func suggestAltCoreForTransport(proto, transport, excludeKind string) string {
	for _, kind := range []string{models.CoreKindSingBox, models.CoreKindMihomo, models.CoreKindXray, models.CoreKindHysteria2} {
		if kind == excludeKind {
			continue
		}
		if coreinfo.SupportsProtocol(kind, proto) && coreinfo.SupportsTransport(kind, transport) {
			return fmt.Sprintf("Switch to %s", coreinfo.GetInfo(kind).Name)
		}
	}
	return fmt.Sprintf("No core found that supports %s + %s", proto, transport)
}
