package api

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/go-playground/validator/v10"
)

// =============================================================================
// Validation Registration
// =============================================================================

// RegisterCustomValidations registers all custom validators with the validator instance.
func RegisterCustomValidations(v *validator.Validate) error {
	// Use JSON tag names in validation error messages instead of struct field names.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return fld.Name
		}
		if name == "" {
			return toSnakeCase(fld.Name)
		}
		return name
	})

	registrations := []func(*validator.Validate) error{
		registerGeneralValidators,
		registerCashPilotXValidators,
	}

	for _, register := range registrations {
		if err := register(v); err != nil {
			return err
		}
	}
	return nil
}

// registerGeneralValidators registers generic utility validators.
func registerGeneralValidators(v *validator.Validate) error {
	return v.RegisterValidation("url", isURL)
}

// registerCashPilotXValidators registers validators that match the enums and
// CHECK constraints defined in the CashPilotX migrations.
func registerCashPilotXValidators(v *validator.Validate) error {
	validators := map[string]func(fl validator.FieldLevel) bool{
		// auth / users
		"phone":    validatePhone,
		"pin":      validatePIN,
		"currency": validateCurrencyCode,

		// invoices / payments
		"invoice_status":   validateInvoiceStatus,
		"collection_state": validateCollectionState,
		"payment_method":   validatePaymentMethod,

		// messages / reminders
		"message_tone":    validateMessageTone,
		"message_channel": validateMessageChannel,
	}

	for name, fn := range validators {
		if err := v.RegisterValidation(name, fn); err != nil {
			return fmt.Errorf("failed to register '%s' validator: %w", name, err)
		}
	}
	return nil
}

// =============================================================================
// General Validators
// =============================================================================

func isURL(fl validator.FieldLevel) bool {
	u, err := url.Parse(fl.Field().String())
	return err == nil && u.Scheme != "" && u.Host != ""
}

// =============================================================================
// Auth / User Validators
// =============================================================================

// phoneRE matches the CHECK constraint: phone ~ '^\+?[0-9]{8,15}$'
var phoneRE = regexp.MustCompile(`^\+?\d{8,15}$`)

// validatePhone mirrors the users.phone CHECK constraint.
func validatePhone(fl validator.FieldLevel) bool {
	return phoneRE.MatchString(fl.Field().String())
}

// validatePIN validates a 6-digit numeric PIN used for login.
var pinRE = regexp.MustCompile(`^\d{6}$`)

func validatePIN(fl validator.FieldLevel) bool {
	return pinRE.MatchString(fl.Field().String())
}

// validateCurrencyCode mirrors the CHECK (char_length(currency) = 3) constraints
// on invoices and payments. Accepts 3-letter ISO 4217 codes.
var currencyRE = regexp.MustCompile(`^[A-Z]{3}$`)

func validateCurrencyCode(fl validator.FieldLevel) bool {
	return currencyRE.MatchString(fl.Field().String())
}

// =============================================================================
// Invoice / Payment Validators
// =============================================================================

// validateInvoiceStatus mirrors the invoice_status_check constraint.
func validateInvoiceStatus(fl validator.FieldLevel) bool {
	return slices.Contains(
		[]string{"unpaid", "partial", "paid", "canceled"},
		fl.Field().String(),
	)
}

// validateCollectionState mirrors the invoice_collection_state_check constraint.
func validateCollectionState(fl validator.FieldLevel) bool {
	return slices.Contains(
		[]string{"pending", "reminded", "no_response", "negotiating", "promised", "paid", "defaulted"},
		fl.Field().String(),
	)
}

// validatePaymentMethod mirrors the payments method CHECK constraint.
func validatePaymentMethod(fl validator.FieldLevel) bool {
	return slices.Contains(
		[]string{"cash", "transfer", "promptpay", "card", "other"},
		fl.Field().String(),
	)
}

// =============================================================================
// Message / Reminder Validators
// =============================================================================

// validateMessageTone mirrors the message_tone enum.
// 'final' is intentionally excluded — it is reserved for system use only.
func validateMessageTone(fl validator.FieldLevel) bool {
	return slices.Contains(
		[]string{"polite", "firm", "urgent"},
		fl.Field().String(),
	)
}

// validateMessageChannel mirrors the message_channel enum.
func validateMessageChannel(fl validator.FieldLevel) bool {
	return slices.Contains(
		[]string{"line", "messenger", "wechat", "telegram", "whatsapp", "external"},
		fl.Field().String(),
	)
}
